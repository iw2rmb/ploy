package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// completeJobRequest represents the request body for job completion.
// This is a simpler contract than the node-based endpoint since job_id
// is in the URL path and node identity comes from mTLS.
type completeJobRequest struct {
	Status   string          `json:"status"`              // Terminal status: Success, Fail, or Cancelled
	ExitCode *int32          `json:"exit_code,omitempty"` // Exit code from job execution
	Stats    json.RawMessage `json:"stats,omitempty"`     // Optional job statistics (must be JSON object)
}

// JobStatsPayload is the typed structure for the stats field in job completion.
// This replaces untyped map[string]any decoding at the API boundary, providing
// schema control over incoming stats payloads.
//
// Wire format example:
//
//	{
//	  "job_meta": { "kind": "gate", "gate": { ... } },
//	  "metadata": { "mr_url": "https://..." },
//	  "duration_ms": 1234
//	}
//
// The job_meta field, when present, must be valid per contracts.UnmarshalJobMeta.
// The metadata field contains string key-value pairs for run-level metadata merging.
type JobStatsPayload struct {
	// JobMeta is the structured gate/build/mod metadata to persist in jobs.meta JSONB.
	// When present, it is validated via contracts.UnmarshalJobMeta before persisting.
	// Empty/null values are treated as "no job meta" (not persisted).
	JobMeta json.RawMessage `json:"job_meta,omitempty"`

	// Metadata contains optional string key-value pairs for run-level context.
	// The mr_url key is used by MR jobs to report merge request URLs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// DurationMs is the job execution duration in milliseconds (informational).
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// MRURL returns the merge request URL from metadata, if present.
// Returns empty string if metadata is nil or mr_url key is absent/empty.
func (p JobStatsPayload) MRURL() string {
	if p.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(p.Metadata["mr_url"])
}

// HasJobMeta returns true if job_meta is present and non-empty.
// Empty JSON objects ("{}") and null are treated as "no job meta".
func (p JobStatsPayload) HasJobMeta() bool {
	trimmed := bytes.TrimSpace(p.JobMeta)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	if bytes.Equal(trimmed, []byte("{}")) {
		return false
	}

	// Treat any empty object form as "no job meta", even if whitespace is present ("{ }").
	// This keeps the API forgiving for clients that emit pretty-printed JSON.
	if trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil && len(obj) == 0 {
			return false
		}
	}
	return true
}

// ValidateJobMeta validates the job_meta field using contracts.UnmarshalJobMeta.
// Returns nil if job_meta is absent/empty or if it passes validation.
// Returns an error describing the validation failure if job_meta is invalid.
func (p JobStatsPayload) ValidateJobMeta() error {
	trimmed := bytes.TrimSpace(p.JobMeta)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	// Treat any empty object form as "no job meta", even if whitespace is present ("{ }").
	if trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil && len(obj) == 0 {
			return nil
		}
	}

	// Use the canonical JobMeta unmarshaler for structural validation.
	// This ensures the job_meta adheres to the contracts.JobMeta schema
	// (valid kind, consistent gate/build metadata presence, etc.).
	if _, err := contracts.UnmarshalJobMeta(trimmed); err != nil {
		return fmt.Errorf("invalid job_meta: %w", err)
	}
	return nil
}

// formatStackGateError formats a Stack Gate failure for run_repos.last_error.
// Returns nil if job meta doesn't contain a stack gate failure.
func formatStackGateError(modType domaintypes.ModType, jobMeta json.RawMessage) *string {
	if len(jobMeta) == 0 {
		return nil
	}
	meta, err := contracts.UnmarshalJobMeta(jobMeta)
	if err != nil || meta.Kind != "gate" || meta.Gate == nil || meta.Gate.StackGate == nil {
		return nil
	}
	sg := meta.Gate.StackGate
	if sg.Result == "pass" {
		return nil
	}

	// Derive phase from mod_type
	phase := "unknown"
	switch modType {
	case domaintypes.ModTypePreGate:
		phase = "inbound"
	case domaintypes.ModTypePostGate, domaintypes.ModTypeReGate:
		phase = "outbound"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Stack Gate [%s]: %s\n", phase, sg.Result))

	if sg.Expected != nil {
		sb.WriteString(fmt.Sprintf("  Expected: {language: %s", sg.Expected.Language))
		if sg.Expected.Tool != "" {
			sb.WriteString(fmt.Sprintf(", tool: %s", sg.Expected.Tool))
		}
		if sg.Expected.Release != "" {
			sb.WriteString(fmt.Sprintf(", release: %q", sg.Expected.Release))
		}
		sb.WriteString("}\n")
	}

	if sg.Detected != nil {
		sb.WriteString(fmt.Sprintf("  Detected: {language: %s", sg.Detected.Language))
		if sg.Detected.Tool != "" {
			sb.WriteString(fmt.Sprintf(", tool: %s", sg.Detected.Tool))
		}
		if sg.Detected.Release != "" {
			sb.WriteString(fmt.Sprintf(", release: %q", sg.Detected.Release))
		}
		sb.WriteString("}\n")
	}

	// Extract evidence from LogFindings
	if meta.Gate != nil && len(meta.Gate.LogFindings) > 0 {
		for _, finding := range meta.Gate.LogFindings {
			if finding.Evidence != "" {
				sb.WriteString("  Evidence:\n")
				for _, line := range strings.Split(finding.Evidence, "\n") {
					if line = strings.TrimSpace(line); line != "" {
						sb.WriteString(fmt.Sprintf("    - %s\n", line))
					}
				}
				break
			}
		}
	}

	result := strings.TrimSuffix(sb.String(), "\n")
	return &result
}

// completeJobHandler marks a job as completed with terminal status and stats.
// This endpoint simplifies the node → server contract by addressing jobs directly
// via the URL path (/v1/jobs/{job_id}/complete) rather than requiring run_id and
// step_index in the request body.
//
// Authentication: Node identity is derived from the mTLS client certificate.
// The handler verifies that the job is assigned to the calling node.
//
// Request body:
//
//	{
//	  "status": "Success" | "Fail" | "Cancelled",
//	  "exit_code": 0,
//	  "stats": { ... }
//	}
//
// Response: 204 No Content on success.
func completeJobHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract job_id from URL path parameter using domain type helper.
		jobID, err := domaintypes.ParseJobIDParam(r, "job_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Extract caller identity from context (set by auth middleware).
		_, ok := auth.IdentityFromContext(ctx)
		if !ok {
			http.Error(w, "unauthorized: no identity in context", http.StatusUnauthorized)
			return
		}

		// Decode request body for status, exit_code, and stats with strict validation.
		var req completeJobRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate and convert status to canonical JobStatus type.
		// v1 uses capitalized job status values: Success, Fail, Cancelled.
		if strings.TrimSpace(req.Status) == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}
		normalizedStatus, err := store.ConvertToJobStatus(strings.TrimSpace(req.Status))
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid status: %v", err), http.StatusBadRequest)
			return
		}

		// Validate that status is a terminal job state (Success, Fail, or Cancelled).
		if normalizedStatus != store.JobStatusSuccess &&
			normalizedStatus != store.JobStatusFail &&
			normalizedStatus != store.JobStatusCancelled {
			http.Error(w, fmt.Sprintf("status must be Success, Fail, or Cancelled, got %s", req.Status), http.StatusBadRequest)
			return
		}

		// Validate and parse stats field into typed JobStatsPayload.
		// This replaces untyped map[string]any decoding with a structured approach
		// that provides compile-time type safety and schema validation.
		statsBytes := []byte("{}")
		var statsPayload JobStatsPayload
		if len(req.Stats) > 0 {
			// First, validate that stats is valid JSON.
			if !json.Valid(req.Stats) {
				http.Error(w, "stats field must be valid JSON", http.StatusBadRequest)
				return
			}

			// Verify stats is a JSON object (not array, string, number, etc.).
			// We do a quick type check before unmarshaling into the typed struct.
			var rawCheck json.RawMessage
			if err := json.Unmarshal(req.Stats, &rawCheck); err != nil {
				http.Error(w, "invalid stats JSON", http.StatusBadRequest)
				return
			}
			// Trim whitespace and check first character for object delimiter.
			trimmed := strings.TrimSpace(string(rawCheck))
			if len(trimmed) == 0 || trimmed[0] != '{' {
				http.Error(w, "stats must be a JSON object", http.StatusBadRequest)
				return
			}

			// Unmarshal into typed JobStatsPayload struct.
			// Unknown fields are silently ignored (forward compatibility).
			if err := json.Unmarshal(req.Stats, &statsPayload); err != nil {
				http.Error(w, fmt.Sprintf("invalid stats payload: %v", err), http.StatusBadRequest)
				return
			}
			statsBytes = req.Stats

			// Validate job_meta via contracts.UnmarshalJobMeta when present.
			// This ensures structured metadata conforms to the JobMeta schema
			// before persisting to jobs.meta JSONB.
			if err := statsPayload.ValidateJobMeta(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		// Look up the job by job_id (KSUID-backed).
		job, err := st.GetJob(ctx, jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: get job failed", "job_id", jobID, "err", err)
			return
		}

		// Derive run_id from the job for run completion checks.
		runID := job.RunID

		// Derive node ID from required header. auth middleware already enforces
		// presence for worker-role callers; this handler performs an additional
		// check and uses the value for job ownership validation.
		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			http.Error(w, "invalid PLOY_NODE_UUID header", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the job is in 'Running' status before transitioning to terminal state.
		// v1 uses capitalized status values.
		if job.Status != store.JobStatusRunning {
			http.Error(w, fmt.Sprintf("job status is %s, expected Running", job.Status), http.StatusConflict)
			return
		}

		// Use the validated job status directly (already a JobStatus type).
		jobStatus := normalizedStatus

		// Transition job status to terminal state.
		// Sets finished_at timestamp, duration_ms, and exit_code.
		// When job_meta is present in stats, persist it into jobs.meta JSONB.
		// The job_meta has already been validated via ValidateJobMeta() above.
		if statsPayload.HasJobMeta() {
			err = st.UpdateJobCompletionWithMeta(ctx, store.UpdateJobCompletionWithMetaParams{
				ID:       job.ID,
				Status:   jobStatus,
				ExitCode: req.ExitCode,
				Meta:     statsPayload.JobMeta,
			})
		} else {
			err = st.UpdateJobCompletion(ctx, store.UpdateJobCompletionParams{
				ID:       job.ID,
				Status:   jobStatus,
				ExitCode: req.ExitCode,
			})
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: update failed",
				"job_id", jobID,
				"step_index", job.StepIndex,
				"node_id", nodeIDHeader,
				"err", err,
			)
			return
		}

		slog.Info("job completed",
			"job_id", jobID,
			"step_index", job.StepIndex,
			"node_id", nodeIDHeader,
			"status", jobStatus,
			"exit_code", req.ExitCode,
			"stats_size", len(statsBytes),
		)

		// Fetch the run for post-completion processing.
		run, err := st.GetRun(ctx, runID)
		if err != nil {
			// Log error but don't fail the job completion (job is already marked complete).
			slog.Error("complete job: get run failed", "job_id", jobID, "run_id", runID, "err", err)
		}

		// When a job fails, either:
		// - If it is a gate job, invoke maybeCreateHealingJobs (which may create healing/re-gate
		//   jobs or cancel remaining jobs when healing is not configured or exhausted).
		// - If it is a non-gate job (mod/heal), cancel remaining non-terminal jobs so the run
		//   can reach a terminal state instead of leaving jobs stranded.
		// v1 uses Fail instead of failed.
		if jobStatus == store.JobStatusFail && err == nil {
			modType := domaintypes.ModType(job.ModType)
			if err := modType.Validate(); err != nil {
				slog.Error("complete job: invalid mod_type in job record; treating as non-gate for failure handling",
					"job_id", jobID,
					"mod_type", job.ModType,
					"err", err,
				)
				modType = ""
			}
			switch modType {
			case domaintypes.ModTypeMR:
				// MR jobs are best-effort and must not trigger healing or
				// cancellation of other jobs when they fail.
				slog.Warn("complete job: MR job failed; ignoring for run-level failure handling",
					"job_id", jobID,
					"step_index", job.StepIndex,
				)
			case domaintypes.ModTypePreGate, domaintypes.ModTypePostGate, domaintypes.ModTypeReGate:
				// Set last_error for Stack Gate failures
				if statsPayload.HasJobMeta() {
					if errMsg := formatStackGateError(modType, statsPayload.JobMeta); errMsg != nil {
						if updateErr := st.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
							RunID:     job.RunID,
							RepoID:    job.RepoID,
							LastError: errMsg,
						}); updateErr != nil {
							slog.Error("complete job: failed to set repo last_error",
								"job_id", jobID,
								"repo_id", job.RepoID,
								"err", updateErr,
							)
						}
					}
				}
				if healErr := maybeCreateHealingJobs(ctx, st, run, job.RunID, job.RepoID, job.Attempt, job.StepIndex); healErr != nil {
					slog.Error("complete job: failed to create healing jobs",
						"job_id", jobID,
						"step_index", job.StepIndex,
						"err", healErr,
					)
				}
			default:
				if err := cancelRemainingJobsAfterFailure(ctx, st, job.RunID, job.RepoID, job.Attempt, job.StepIndex); err != nil {
					slog.Error("complete job: failed to cancel remaining jobs after non-gate failure",
						"job_id", jobID,
						"step_index", job.StepIndex,
						"err", err,
					)
				}
			}
		}

		// When a job is cancelled (not by explicit run cancel), ensure the repo attempt
		// reaches a terminal state by cancelling remaining jobs after this step.
		// This is critical for policy-driven cancellations (e.g., stack detection required).
		if jobStatus == store.JobStatusCancelled && err == nil {
			modType := domaintypes.ModType(job.ModType)
			if modType.Validate() == nil && modType == domaintypes.ModTypeMR {
				// MR jobs are best-effort and must not affect run-level progression.
			} else {
				if cerr := cancelRemainingJobsAfterFailure(ctx, st, job.RunID, job.RepoID, job.Attempt, job.StepIndex); cerr != nil {
					slog.Error("complete job: failed to cancel remaining jobs after cancellation",
						"job_id", jobID,
						"step_index", job.StepIndex,
						"err", cerr,
					)
				}
			}
		}

		// Server-driven scheduling: after job succeeds, schedule the next job.
		if jobStatus == store.JobStatusSuccess {
			if _, err := st.ScheduleNextJob(ctx, store.ScheduleNextJobParams{RunID: job.RunID, RepoID: job.RepoID, Attempt: job.Attempt}); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to schedule next job",
						"job_id", jobID,
						"step_index", job.StepIndex,
						"err", err,
					)
				}
			}
		}

		// v1 repo-scoped progression:
		// After completing a job, check if the repo attempt has reached a terminal state.
		// MR jobs (mod_type='mr') are auxiliary and must not affect run_repos.status derivation.
		jobModType := domaintypes.ModType(job.ModType)
		isMRJob := jobModType.Validate() == nil && jobModType == domaintypes.ModTypeMR
		if err == nil && !isMRJob {
			// Update run_repos.status if all jobs for this repo attempt are terminal.
			repoUpdated, repoErr := maybeUpdateRunRepoStatus(ctx, st, job.RunID, job.RepoID, job.Attempt)
			if repoErr != nil {
				slog.Error("complete job: failed to check repo completion",
					"job_id", jobID,
					"repo_id", job.RepoID,
					"attempt", job.Attempt,
					"err", repoErr,
				)
			}

			// If the repo reached terminal state, check if the run should transition to Finished.
			// runs.status becomes Finished when all repos are terminal.
			if repoUpdated {
				if completeErr := maybeCompleteRunIfAllReposTerminal(ctx, st, eventsService, run, runID); completeErr != nil {
					slog.Error("complete job: failed to check run completion",
						"job_id", jobID,
						"step_index", job.StepIndex,
						"err", completeErr,
					)
				}
			}
		}

		// For MR jobs that reported an MR URL in stats.metadata.mr_url, merge the
		// URL into runs.stats.metadata.mr_url so that GET /v1/runs/{id}/status can
		// expose it via RunStats.MRURL() and CLI commands can display it. This is a
		// best-effort update and does not affect run status.
		// We use the typed statsPayload.MRURL() accessor instead of map[string]any casting.
		// Note: jobModType and isMRJob were already computed above for repo-scoped progression.
		mrURL := statsPayload.MRURL()
		if err == nil && mrURL != "" && isMRJob {
			if updateErr := st.UpdateRunStatsMRURL(ctx, store.UpdateRunStatsMRURLParams{
				ID:    runID,
				MrUrl: mrURL,
			}); updateErr != nil {
				slog.Error("complete job: failed to merge MR URL into run stats",
					"job_id", jobID,
					"run_id", runID,
					"err", updateErr,
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
