package handlers

import (
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
)

// completeJobRequest represents the request body for job completion.
// This is a simpler contract than the node-based endpoint since job_id
// is in the URL path and node identity comes from mTLS.
type completeJobRequest struct {
	Status   string          `json:"status"`              // Terminal status: succeeded, failed, or canceled
	ExitCode *int32          `json:"exit_code,omitempty"` // Exit code from job execution
	Stats    json.RawMessage `json:"stats,omitempty"`     // Optional job statistics (must be JSON object)
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
//	  "status": "succeeded" | "failed" | "canceled",
//	  "exit_code": 0,
//	  "stats": { ... }
//	}
//
// Response: 204 No Content on success.
func completeJobHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract job_id from URL path parameter.
		// Job IDs are KSUID strings; treated as opaque identifiers.
		jobIDStr := strings.TrimSpace(r.PathValue("job_id"))
		if jobIDStr == "" {
			http.Error(w, "job_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Extract caller identity from context (set by auth middleware).
		_, ok := auth.IdentityFromContext(ctx)
		if !ok {
			http.Error(w, "unauthorized: no identity in context", http.StatusUnauthorized)
			return
		}

		// Decode request body for status, exit_code, and stats.
		var req completeJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate and convert status to canonical RunStatus type.
		if strings.TrimSpace(req.Status) == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}
		normalizedStatus, err := store.ConvertToRunStatus(strings.ToLower(strings.TrimSpace(req.Status)))
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid status: %v", err), http.StatusBadRequest)
			return
		}

		// Validate that status is a terminal state (succeeded, failed, or canceled).
		if normalizedStatus != store.RunStatusSucceeded &&
			normalizedStatus != store.RunStatusFailed &&
			normalizedStatus != store.RunStatusCanceled {
			http.Error(w, fmt.Sprintf("status must be succeeded, failed, or canceled, got %s", req.Status), http.StatusBadRequest)
			return
		}

		// Validate stats field if provided (must be a valid JSON object).
		statsBytes := []byte("{}")
		if len(req.Stats) > 0 {
			if !json.Valid(req.Stats) {
				http.Error(w, "stats field must be valid JSON", http.StatusBadRequest)
				return
			}
			var tmp any
			if err := json.Unmarshal(req.Stats, &tmp); err != nil {
				http.Error(w, "invalid stats JSON", http.StatusBadRequest)
				return
			}
			if _, ok := tmp.(map[string]any); !ok {
				http.Error(w, "stats must be a JSON object", http.StatusBadRequest)
				return
			}
			statsBytes = req.Stats
		}

		// Look up the job by job_id using string ID directly.
		// No UUID parsing needed; store accepts KSUID strings.
		job, err := st.GetJob(ctx, jobIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: get job failed", "job_id", jobIDStr, "err", err)
			return
		}

		// Derive run_id from the job for run completion checks.
		runID := job.RunID

		// Derive node ID from required header. auth middleware already enforces
		// presence for worker-role callers; this handler performs an additional
		// check and uses the value for job ownership validation.
		// Node IDs are now NanoID(6) strings.
		nodeIDHeader := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeader == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node.
		// job.NodeID is *string after node ID migration.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the job is in 'running' status before transitioning to terminal state.
		if job.Status != store.JobStatusRunning {
			http.Error(w, fmt.Sprintf("job status is %s, expected running", job.Status), http.StatusConflict)
			return
		}

		// Map run terminal status (succeeded/failed/canceled) to JobStatus.
		var jobStatus store.JobStatus
		switch normalizedStatus {
		case store.RunStatusSucceeded:
			jobStatus = store.JobStatusSucceeded
		case store.RunStatusFailed:
			jobStatus = store.JobStatusFailed
		case store.RunStatusCanceled:
			jobStatus = store.JobStatusCanceled
		default:
			jobStatus = store.JobStatusFailed
		}

		// Transition job status to terminal state.
		// Sets finished_at timestamp, duration_ms, and exit_code.
		err = st.UpdateJobCompletion(ctx, store.UpdateJobCompletionParams{
			ID:       job.ID,
			Status:   jobStatus,
			ExitCode: req.ExitCode,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: update failed",
				"job_id", jobIDStr,
				"step_index", job.StepIndex,
				"node_id", nodeIDHeader,
				"err", err,
			)
			return
		}

		slog.Info("job completed",
			"job_id", jobIDStr,
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
			slog.Error("complete job: get run failed", "job_id", jobIDStr, "run_id", runID, "err", err)
		}

		// When a job fails, either:
		// - If it is a gate job, invoke maybeCreateHealingJobs (which may create healing/re-gate
		//   jobs or cancel remaining jobs when healing is not configured or exhausted).
		// - If it is a non-gate job (mod/heal), cancel remaining non-terminal jobs so the run
		//   can reach a terminal state instead of leaving jobs stranded.
		if jobStatus == store.JobStatusFailed && err == nil {
			jobs, jobsErr := st.ListJobsByRun(ctx, runID)
			if jobsErr != nil {
				slog.Error("complete job: failed to list jobs for failure handling",
					"job_id", jobIDStr,
					"err", jobsErr,
				)
			} else {
				modType := strings.TrimSpace(job.ModType)
				if modType == "pre_gate" || modType == "post_gate" || modType == "re_gate" {
					if healErr := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(job.StepIndex), jobs); healErr != nil {
						slog.Error("complete job: failed to create healing jobs",
							"job_id", jobIDStr,
							"step_index", job.StepIndex,
							"err", healErr,
						)
					}
				} else {
					if err := cancelRemainingJobsAfterFailure(ctx, st, runID, domaintypes.StepIndex(job.StepIndex), jobs); err != nil {
						slog.Error("complete job: failed to cancel remaining jobs after non-gate failure",
							"job_id", jobIDStr,
							"step_index", job.StepIndex,
							"err", err,
						)
					}
				}
			}
		}

		// Server-driven scheduling: after job succeeds or is skipped, schedule the next job.
		if jobStatus == store.JobStatusSucceeded || jobStatus == store.JobStatusSkipped {
			if _, err := st.ScheduleNextJob(ctx, runID); err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to schedule next job",
						"job_id", jobIDStr,
						"step_index", job.StepIndex,
						"err", err,
					)
				}
			}
		}

		// After completing a job, check if the run should transition to terminal state.
		if err == nil {
			if completeErr := maybeCompleteMultiStepRun(ctx, st, eventsService, run, runID); completeErr != nil {
				slog.Error("complete job: failed to check run completion",
					"job_id", jobIDStr,
					"step_index", job.StepIndex,
					"err", completeErr,
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
