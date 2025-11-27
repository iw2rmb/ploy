package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// claimJobHandler allows nodes to claim a pending job for execution.
// Returns the assigned job with its parent run metadata or 204 No Content if no work is available.
//
// Jobs are the unified execution unit for all work types: pre-gate, mod, heal, re-gate, post-gate.
// Jobs are ordered by step_index (FLOAT) to support dynamic insertion of healing jobs.
func claimJobHandler(st store.Store, configHolder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeID := domaintypes.ToPGUUID(nodeIDStr)
		if !nodeID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
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

		// Claim the next pending job.
		job, err := st.ClaimJob(r.Context(), nodeID)
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

		// Fetch parent run metadata.
		run, err := st.GetRun(r.Context(), job.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get run for claimed job: %v", err), http.StatusInternalServerError)
			slog.Error("claim: get run failed for job", "node_id", nodeIDStr, "job_id", uuid.UUID(job.ID.Bytes).String(), "err", err)
			return
		}

		// Transition run to 'running' if it's still queued.
		if run.Status == store.RunStatusQueued {
			if err := st.AckRunStart(r.Context(), run.ID); err != nil {
				slog.Warn("claim: failed to ack run start", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			}
		}

		// Build and send response with job and run information.
		buildAndSendJobClaimResponse(w, r, configHolder, run, job)
		slog.Info("job claimed",
			"job_id", uuid.UUID(job.ID.Bytes).String(),
			"job_name", job.Name,
			"run_id", uuid.UUID(run.ID.Bytes).String(),
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
	jobIDStr := uuid.UUID(job.ID.Bytes).String()
	mergedSpec := mergeJobIDIntoSpec(run.Spec, jobIDStr)

	// Merge server default GitLab config (token/domain) into spec if configured.
	// Per-run overrides (already in spec) take precedence over server defaults.
	gitlabCfg := configHolder.GetGitLab()
	mergedSpec = mergeGitLabConfigIntoSpec(mergedSpec, gitlabCfg)

	resp := struct {
		ID        string          `json:"id"`         // Run ID
		JobID     string          `json:"job_id"`     // Job ID
		JobName   string          `json:"job_name"`   // Job name (e.g., "pre-gate", "mod-0")
		StepIndex float64         `json:"step_index"` // Job ordering index
		RepoURL   string          `json:"repo_url"`
		Status    store.RunStatus `json:"status"`
		NodeID    string          `json:"node_id"`
		BaseRef   string          `json:"base_ref"`
		TargetRef string          `json:"target_ref"`
		CommitSha *string         `json:"commit_sha,omitempty"`
		StartedAt string          `json:"started_at"`
		CreatedAt string          `json:"created_at"`
		Spec      json.RawMessage `json:"spec,omitempty"`
	}{
		ID:        uuid.UUID(run.ID.Bytes).String(),
		JobID:     jobIDStr,
		JobName:   job.Name,
		StepIndex: job.StepIndex,
		RepoURL:   run.RepoUrl,
		Status:    run.Status,
		NodeID:    uuid.UUID(job.NodeID.Bytes).String(),
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
