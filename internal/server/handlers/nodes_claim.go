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
	"github.com/iw2rmb/ploy/internal/store"
)

// claimJobHandler allows nodes to claim a pending job for execution.
// Returns the claimed job with its parent run metadata or 204 No Content if no work is available.
//
// Server-driven scheduling: only 'pending' jobs can be claimed. When a job completes,
// the server schedules the next 'created' job by transitioning it to 'pending'.
//
// Jobs are the unified execution unit for all work types: pre-gate, mod, heal, re-gate, post-gate.
// Jobs are ordered by step_index (FLOAT) to support dynamic insertion of healing jobs.
// Jobs transition directly from 'pending' to 'running' on claim (no 'assigned' intermediate state).
func claimJobHandler(st store.Store, configHolder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Node IDs are now NanoID(6) strings; no UUID parsing needed.
		nodeID := strings.TrimSpace(nodeIDStr)
		if nodeID == "" {
			http.Error(w, "invalid id: must be a non-empty string", http.StatusBadRequest)
			return
		}

		var err error
		// Verify node exists before attempting to claim work.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("claim: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Claim the next pending job. ClaimJob expects *string for nullable FK.
		job, err := st.ClaimJob(r.Context(), &nodeID)
		if err != nil {
			// No pending jobs available; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim: no work available", "node_id", nodeIDStr)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: database error", "node_id", nodeIDStr, "err", err)
			return
		}

		// Fetch parent run metadata. Run IDs are KSUID strings.
		run, err := st.GetRun(r.Context(), job.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get run for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get run failed for job", "node_id", nodeIDStr, "job_id", job.ID, "err", err)
			return
		}

		// Transition run to 'running' if it's still queued.
		if run.Status == store.RunStatusQueued {
			if err := st.AckRunStart(r.Context(), run.ID); err != nil {
				slog.Warn("claim: failed to ack run start", "run_id", run.ID, "err", err)
			}
		}

		// Build and send response with job and run information.
		buildAndSendJobClaimResponse(w, r, configHolder, run, job)
		slog.Info("job claimed",
			"job_id", job.ID, // Job IDs are KSUID strings.
			"job_name", job.Name,
			"run_id", run.ID, // Run IDs are KSUID strings.
			"step_index", job.StepIndex,
			"node_id", nodeIDStr,
		)
	}
}

// buildAndSendJobClaimResponse constructs and sends the claim response for a job.
func buildAndSendJobClaimResponse(
	w http.ResponseWriter,
	r *http.Request,
	configHolder *ConfigHolder,
	run store.Run,
	job store.Job,
) {
	// Merge job_id into spec for downstream execution.
	// Job IDs are now KSUID strings.
	mergedSpec := mergeJobIDIntoSpec(run.Spec, job.ID)

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

	resp := struct {
		ID        string                `json:"id"`         // Run ID
		JobID     string                `json:"job_id"`     // Job ID
		JobName   string                `json:"job_name"`   // Job name (e.g., "pre-gate", "mod-0")
		ModType   string                `json:"mod_type"`   // Job phase: pre_gate, mod, post_gate, heal, re_gate
		ModImage  string                `json:"mod_image"`  // Container image for mod/heal jobs
		StepIndex domaintypes.StepIndex `json:"step_index"` // Job ordering index
		RepoURL   string                `json:"repo_url"`
		Status    store.RunStatus       `json:"status"`
		NodeID    string                `json:"node_id"`
		BaseRef   string                `json:"base_ref"`
		TargetRef string                `json:"target_ref"`
		CommitSha *string               `json:"commit_sha,omitempty"`
		StartedAt string                `json:"started_at"`
		CreatedAt string                `json:"created_at"`
		Spec      json.RawMessage       `json:"spec,omitempty"`
	}{
		ID:        run.ID, // Run IDs are KSUID strings.
		JobID:     job.ID, // Job IDs are KSUID strings.
		JobName:   job.Name,
		ModType:   job.ModType,
		ModImage:  job.ModImage,
		StepIndex: domaintypes.StepIndex(job.StepIndex),
		RepoURL:   run.RepoUrl,
		Status:    run.Status,
		NodeID:    stringPtrOrEmpty(job.NodeID), // Node IDs are NanoID strings (nullable).
		BaseRef:   run.BaseRef,
		TargetRef: run.TargetRef,
		CommitSha: run.CommitSha,
		StartedAt: run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
		Spec:      mergedSpec,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("claim: encode response failed", "err", err)
	}
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
