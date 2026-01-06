package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// claimJobHandler allows nodes to claim a queued job for execution.
// Returns the claimed job with its parent run metadata or 204 No Content if no work is available.
//
// v1 status rules (per roadmap/v1/statuses.md):
// - claimable jobs have status='Queued'; claimed jobs transition to 'Running'
// - normal jobs are claimable only when runs.status='Started'
// - MR jobs (mod_type='mr') are claimable only when runs.status='Finished'
// - on first claim for a repo attempt, run_repos.status transitions Queued → Running
// - repo progression is attempt-scoped (run_id, repo_id, attempt)
//
// v1 response includes repo attribution (per roadmap/v1/scope.md:84):
// - repo_url: from mod_repos (since runs no longer have repo_url fields)
// - base_ref: from jobs.repo_base_ref (snapshot at job creation)
// - target_ref: from run_repos.repo_target_ref (snapshot at run_repos creation)
//
// Jobs are claimed from a single unified queue (FIFO by step_index). There is no
// separate Build Gate queue or claim path — all job types (pre-gate, mod, heal,
// re-gate, post-gate) are consumed from the same queue.
// Jobs are ordered by step_index (FLOAT) to support dynamic insertion of healing jobs.
// Jobs transition directly from 'Queued' to 'Running' on claim (no intermediate state).
func claimJobHandler(st store.Store, configHolder *ConfigHolder, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeID, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to claim work.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("claim: node check failed", "node_id", nodeID, "err", err)
			return
		}

		// Claim the next pending job. ClaimJob expects *string for nullable FK.
		job, err := st.ClaimJob(r.Context(), &nodeID)
		if err != nil {
			// No pending jobs available; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim: no work available", "node_id", nodeID)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: database error", "node_id", nodeID, "err", err)
			return
		}

		// v1 repo status transition: Queued → Running on first claim for repo attempt.
		// Per roadmap/v1/statuses.md:84, this is idempotent (already Running repos stay Running).
		// The UpdateRunRepoStatus query sets started_at on first transition to Running.
		_ = st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
			RunID:  job.RunID,
			RepoID: job.RepoID,
			Status: store.RunRepoStatusRunning,
		})

		run, err := st.GetRun(r.Context(), job.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get run for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get run failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: job.RunID, RepoID: job.RepoID})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get run repo for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get run repo failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		modRepo, err := st.GetModRepo(r.Context(), job.RepoID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get repo for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get mod repo failed for job", "node_id", nodeID, "job_id", job.ID, "repo_id", job.RepoID, "err", err)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get spec for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get spec failed for job", "node_id", nodeID, "job_id", job.ID, "spec_id", run.SpecID, "err", err)
			return
		}

		// Build and send response with job and run information.
		if err := buildAndSendJobClaimResponse(w, r, configHolder, run, spec.Spec, rr, modRepo, job); err != nil {
			slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
			http.Error(w, fmt.Sprintf("failed to build claim response: %v", err), http.StatusInternalServerError)
			return
		}
		slog.Info("job claimed",
			"job_id", job.ID, // Job IDs are KSUID strings.
			"job_name", job.Name,
			"run_id", run.ID, // Run IDs are KSUID strings.
			"step_index", job.StepIndex,
			"node_id", nodeID,
		)
	}
}

// buildAndSendJobClaimResponse constructs and sends the claim response for a job.
func buildAndSendJobClaimResponse(
	w http.ResponseWriter,
	r *http.Request,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	runRepo store.RunRepo,
	modRepo store.ModRepo,
	job store.Job,
) error {
	modType := domaintypes.ModType(job.ModType)
	if err := modType.Validate(); err != nil {
		return fmt.Errorf("invalid claimed job mod_type %q for job_id=%s: %w", job.ModType, job.ID, err)
	}

	// Merge job_id into spec for downstream execution.
	// Job IDs are now KSUID strings.
	mergedSpec := mergeJobIDIntoSpec(spec, job.ID)

	// For mod jobs (names following "mod-N" pattern), inject a numeric
	// mod_index derived from job name so the node agent can map this job
	// to mods[N] in multi-step specs.
	if strings.HasPrefix(job.Name, "mod-") {
		if idx, err := parseModIndex(job.Name); err == nil {
			mergedSpec = mergeModIndexIntoSpec(mergedSpec, idx)
		}
	}

	// Merge server default GitLab config (token/domain) into spec if configured.
	// Per-run overrides (already in spec) take precedence over server defaults.
	gitlabCfg := configHolder.GetGitLab()
	mergedSpec = mergeGitLabConfigIntoSpec(mergedSpec, gitlabCfg)

	// Merge global env vars (CA_CERTS_PEM_BUNDLE, CODEX_AUTH_JSON, OPENAI_API_KEY, etc.)
	// into spec.env based on job type and scope matching.
	// Per-run env vars in spec take precedence over global env.
	mergedSpec = mergeGlobalEnvIntoSpec(mergedSpec, configHolder.GetGlobalEnv(), modType)

	// Response uses domain types for type-safe API output.
	// RunID uses JSON key "id" for wire compatibility with existing clients.
	resp := struct {
		RunID     domaintypes.RunID     `json:"id"` // Run ID (KSUID); JSON key stays "id" for wire compatibility
		Name      *string               `json:"name,omitempty"`
		RepoID    string                `json:"repo_id"`
		Attempt   int32                 `json:"attempt"`
		JobID     domaintypes.JobID     `json:"job_id"`     // Job ID (KSUID-backed)
		JobName   string                `json:"job_name"`   // Job name (e.g., "pre-gate", "mod-0")
		ModType   domaintypes.ModType   `json:"mod_type"`   // Job phase: pre_gate, mod, post_gate, heal, re_gate
		ModImage  string                `json:"mod_image"`  // Container image for mod/heal jobs
		StepIndex domaintypes.StepIndex `json:"step_index"` // Job ordering index
		RepoURL   string                `json:"repo_url"`
		Status    store.RunStatus       `json:"status"`
		NodeID    domaintypes.NodeID    `json:"node_id"` // Node ID (NanoID-backed)
		BaseRef   string                `json:"base_ref"`
		TargetRef string                `json:"target_ref"`
		StartedAt string                `json:"started_at"`
		CreatedAt string                `json:"created_at"`
		Spec      json.RawMessage       `json:"spec,omitempty"`
	}{
		RunID:     domaintypes.RunID(run.ID), // Convert to domain type
		Name:      nil,
		RepoID:    job.RepoID,
		Attempt:   job.Attempt,
		JobID:     domaintypes.JobID(job.ID), // Convert to domain type
		JobName:   job.Name,
		ModType:   modType,
		ModImage:  job.ModImage,
		StepIndex: domaintypes.StepIndex(job.StepIndex),
		RepoURL:   modRepo.RepoUrl,
		Status:    run.Status,
		NodeID:    domaintypes.NodeID(stringPtrOrEmpty(job.NodeID)), // Convert to domain type
		BaseRef:   job.RepoBaseRef,
		TargetRef: runRepo.RepoTargetRef,
		StartedAt: run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
		Spec:      mergedSpec,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("claim: encode response failed", "err", err)
	}
	return nil
}

// mergeJobIDIntoSpec injects job_id into the spec JSONB for downstream execution.
func mergeJobIDIntoSpec(spec []byte, jobID string) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(spec, &m); err != nil {
		m = make(map[string]interface{})
	}
	m["job_id"] = jobID
	merged, _ := json.Marshal(m)
	return merged
}

// mergeModIndexIntoSpec injects mod_index into the spec JSONB for downstream execution.
// mod_index maps a mod job (mod-N) to mods[N] in multi-step specs.
func mergeModIndexIntoSpec(spec []byte, modIndex int) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(spec, &m); err != nil {
		m = make(map[string]interface{})
	}
	m["mod_index"] = modIndex
	merged, _ := json.Marshal(m)
	return merged
}

// parseModIndex extracts the numeric index from a mod job name ("mod-N").
func parseModIndex(name string) (int, error) {
	const prefix = "mod-"
	if !strings.HasPrefix(name, prefix) {
		return 0, fmt.Errorf("job name %q does not use mod-N pattern", name)
	}
	suffix := strings.TrimPrefix(name, prefix)
	if strings.TrimSpace(suffix) == "" {
		return 0, fmt.Errorf("empty mod index in job name %q", name)
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, fmt.Errorf("parse mod index from %q: %w", name, err)
	}
	return idx, nil
}

// stringPtrOrEmpty dereferences a *string, returning empty string if nil.
// Used for nullable TEXT FK fields like node_id.
func stringPtrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
