package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// claimJobHandler allows nodes to claim a queued job for execution.
// Returns the claimed job with its parent run metadata or 204 No Content if no work is available.
//
// v1 status rules:
// - claimable jobs have status='Queued'; claimed jobs transition to 'Running'
// - normal jobs are claimable only when runs.status='Started'
// - MR jobs (mod_type='mr') are claimable only when runs.status='Finished'
// - on first claim for a repo attempt, run_repos.status transitions Queued → Running
// - repo progression is attempt-scoped (run_id, repo_id, attempt)
//
// v1 response includes repo attribution:
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
		// Extract node id from path parameter using domain type helper.
		nodeID, err := parseParam[domaintypes.NodeID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify node exists before attempting to claim work.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check node: %v", err)
			slog.Error("claim: node check failed", "node_id", nodeID, "err", err)
			return
		}

		// Claim the next pending job. ClaimJob requires a non-empty nodeID.
		job, err := st.ClaimJob(r.Context(), nodeID)
		if err != nil {
			// No pending jobs available; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim: no work available", "node_id", nodeID)
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to claim job: %v", err)
			slog.Error("claim: database error", "node_id", nodeID, "err", err)
			return
		}

		run, err := st.GetRun(r.Context(), job.RunID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get run for claimed job: %v", err)
			slog.Error("claim: get run failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: job.RunID, RepoID: job.RepoID})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get run repo for claimed job: %v", err)
			slog.Error("claim: get run repo failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		// v1 repo status transition: Queued → Running on first claim for repo attempt.
		// This is idempotent (already Running repos stay Running).
		// MR jobs must not affect run_repos.status.
		isMRJob := job.ModType == domaintypes.ModTypeMR.String()
		if !isMRJob && rr.Status == store.RunRepoStatusQueued {
			// The UpdateRunRepoStatus query sets started_at on first transition to Running.
			if err := st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
				RunID:  job.RunID,
				RepoID: job.RepoID,
				Status: store.RunRepoStatusRunning,
			}); err != nil {
				slog.Error("claim: failed to transition run repo to Running", "node_id", nodeID, "job_id", job.ID, "run_id", job.RunID, "repo_id", job.RepoID, "err", err)
			}
		}

		modRepo, err := st.GetModRepo(r.Context(), job.RepoID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get repo for claimed job: %v", err)
			slog.Error("claim: get mod repo failed for job", "node_id", nodeID, "job_id", job.ID, "repo_id", job.RepoID, "err", err)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get spec for claimed job: %v", err)
			slog.Error("claim: get spec failed for job", "node_id", nodeID, "job_id", job.ID, "spec_id", run.SpecID, "err", err)
			return
		}

		// Build and send response with job and run information.
		if err := buildAndSendJobClaimResponse(w, r, configHolder, run, spec.Spec, rr, modRepo, job); err != nil {
			slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
			httpErr(w, http.StatusInternalServerError, "failed to build claim response: %v", err)
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

	stepIndex := domaintypes.StepIndex(job.StepIndex)
	if !stepIndex.Valid() {
		return fmt.Errorf("invalid step_index for job_id=%s", job.ID)
	}

	// Merge job_id into spec for downstream execution.
	// Job IDs are now KSUID strings.
	mergedSpec, err := mergeJobIDIntoSpec(spec, job.ID)
	if err != nil {
		return fmt.Errorf("merge job_id into spec: %w", err)
	}

	// Merge server default GitLab config (token/domain) into spec if configured.
	// Per-run overrides (already in spec) take precedence over server defaults.
	gitlabCfg := configHolder.GetGitLab()
	mergedSpec, err = mergeGitLabConfigIntoSpec(mergedSpec, gitlabCfg)
	if err != nil {
		return fmt.Errorf("merge gitlab defaults into spec: %w", err)
	}

	// Merge global env vars (CA_CERTS_PEM_BUNDLE, CODEX_AUTH_JSON, OPENAI_API_KEY, etc.)
	// into spec.env based on job type and scope matching.
	// Per-run env vars in spec take precedence over global env.
	mergedSpec, err = mergeGlobalEnvIntoSpec(mergedSpec, configHolder.GetGlobalEnv(), modType)
	if err != nil {
		return fmt.Errorf("merge global env into spec: %w", err)
	}

	// Response uses domain types for type-safe API output.
	// RunID uses JSON key "id" for wire compatibility with existing clients.
	resp := struct {
		RunID     domaintypes.RunID     `json:"id"` // Run ID (KSUID); JSON key stays "id" for wire compatibility
		Name      *string               `json:"name,omitempty"`
		RepoID    domaintypes.ModRepoID `json:"repo_id"`
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
		RunID:     run.ID,
		Name:      nil,
		RepoID:    job.RepoID,
		Attempt:   job.Attempt,
		JobID:     job.ID,
		JobName:   job.Name,
		ModType:   modType,
		ModImage:  job.ModImage,
		StepIndex: stepIndex,
		RepoURL:   modRepo.RepoUrl,
		Status:    run.Status,
		NodeID:    nodeIDPtrOrZero(job.NodeID),
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
func mergeJobIDIntoSpec(spec []byte, jobID domaintypes.JobID) (json.RawMessage, error) {
	m, err := parseSpecObjectStrict(json.RawMessage(spec))
	if err != nil {
		return nil, err
	}
	m["job_id"] = jobID.String()
	merged, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return merged, nil
}

func nodeIDPtrOrZero(id *domaintypes.NodeID) domaintypes.NodeID {
	if id == nil {
		return ""
	}
	return *id
}

// --- Functions merged from spec_utils.go ---

func parseSpecObjectStrict(spec json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(spec)) == 0 {
		return nil, fmt.Errorf("spec: expected JSON object, got empty")
	}

	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, fmt.Errorf("spec: expected JSON object: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("spec: expected JSON object, got null")
	}
	return m, nil
}

// mergeGlobalEnvIntoSpec injects global environment variables into the spec's "env" map.
// Global env vars are only merged if their scope matches the job type.
// Per-run env vars (already in spec) take precedence over global env — existing keys
// are not overwritten.
//
// Parameters:
//   - spec: The job spec JSON, may contain an "env" map
//   - env: Map of global env vars from ConfigHolder (uses typed GlobalEnvScope)
//   - modType: The job's mod_type as typed enum (pre_gate, mod, post_gate, heal, re_gate, mr)
//
// Returns the modified spec with global env vars merged into the "env" field.
func mergeGlobalEnvIntoSpec(spec json.RawMessage, env map[string]GlobalEnvVar, modType domaintypes.ModType) (json.RawMessage, error) {
	// If no global env vars exist, return spec unchanged.
	if len(env) == 0 {
		return spec, nil
	}

	// Parse the spec JSON into an object map.
	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	// Extract existing env map from spec, or create empty one.
	var em map[string]any
	if v, ok := m["env"]; ok && v != nil {
		var ok2 bool
		em, ok2 = v.(map[string]any)
		if !ok2 {
			return nil, fmt.Errorf("spec.env: expected object, got %T", v)
		}
	} else {
		em = map[string]any{}
	}

	// Merge global env vars that match the job scope.
	// Per-run env vars take precedence — skip keys that already exist.
	for k, v := range env {
		// Check if this global env var's typed scope matches the job type.
		// The scope matching uses typed enums to prevent typo-class bugs.
		if !v.Scope.MatchesModType(modType) {
			continue
		}
		// Per-run env wins over global; do not overwrite existing keys.
		if _, exists := em[k]; exists {
			continue
		}
		em[k] = v.Value
	}

	// Update the spec with merged env and serialize back to JSON.
	m["env"] = em
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}

// mergeGitLabConfigIntoSpec merges GitLab default token and domain into the JSON spec payload.
// Only merges values if they are non-empty and not already present in the spec.
// Per-run values (already in spec) take precedence over server defaults.
func mergeGitLabConfigIntoSpec(spec json.RawMessage, cfg config.GitLabConfig) (json.RawMessage, error) {
	// If config is empty, return spec unchanged.
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	// Only add server defaults if per-run overrides are not present.
	if _, hasPerRunPAT := m["gitlab_pat"]; !hasPerRunPAT && cfg.Token != "" {
		m["gitlab_pat"] = cfg.Token
	}
	if _, hasPerRunDomain := m["gitlab_domain"]; !hasPerRunDomain && cfg.Domain != "" {
		m["gitlab_domain"] = cfg.Domain
	}

	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}
