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
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// completeRunHandler marks a run as completed with terminal status and stats.
// Sets finished_at timestamp and populates runs.stats field.
func completeRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id, job_id, status, exit_code, stats, and step_index.
		// Nodeagent includes job_id to identify which job is being completed (avoids float equality issues).
		// step_index is retained for logging/diagnostics but job_id is the authoritative lookup key.
		var req struct {
			RunID     domaintypes.RunID     `json:"run_id"`
			JobID     domaintypes.JobID     `json:"job_id"` // Job ID for completion (authoritative lookup key)
			Status    string                `json:"status"`
			ExitCode  *int32                `json:"exit_code,omitempty"` // Exit code from job execution
			Stats     json.RawMessage       `json:"stats,omitempty"`
			StepIndex domaintypes.StepIndex `json:"step_index"` // Retained for logging/compat
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if req.RunID.IsZero() {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate job_id is present (required for job lookup).
		if req.JobID.IsZero() {
			http.Error(w, "job_id is required", http.StatusBadRequest)
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

		// Verify node exists before attempting to complete the run.
		_, err = st.GetNode(r.Context(), nodeIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists.
		run, err := st.GetRun(r.Context(), req.RunID.String())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Prepare stats field (default to empty JSON object if not provided).
		// Stats validation is shared between run-level and step-level completions.
		statsBytes := []byte("{}")
		if len(req.Stats) > 0 {
			// Validate that stats is valid JSON and a JSON object.
			if !json.Valid(req.Stats) {
				http.Error(w, "stats field must be valid JSON", http.StatusBadRequest)
				return
			}
			// Require JSON object for stats (not string/number/array/etc.).
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

		// Job-level completion: retrieve the job by job_id and transition it to terminal state.
		// Using job_id avoids float equality issues with step_index.
		job, err := st.GetJob(r.Context(), req.JobID.String())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: get job failed", "run_id", req.RunID, "job_id", req.JobID, "err", err)
			return
		}

		// Verify the job belongs to the specified run.
		if job.RunID != req.RunID.String() {
			http.Error(w, "job does not belong to this run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the requesting node.
		if job.NodeID == nil || *job.NodeID != nodeIDStr {
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
			// Fallback for unexpected terminal states.
			jobStatus = store.JobStatusFailed
		}

		// Transition job status to terminal state (succeeded/failed/canceled).
		// Sets finished_at timestamp, duration_ms, and exit_code.
		err = st.UpdateJobCompletion(r.Context(), store.UpdateJobCompletionParams{
			ID:       job.ID,
			Status:   jobStatus,
			ExitCode: req.ExitCode,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: update failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", job.StepIndex, "node_id", nodeIDStr, "err", err)
			return
		}

		slog.Info("job completed",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"step_index", job.StepIndex,
			"node_id", nodeIDStr,
			"status", jobStatus,
			"exit_code", req.ExitCode,
			"stats_size", len(statsBytes),
		)

		// When a job fails, either:
		// - If it is a gate job, invoke maybeCreateHealingJobs (which may create healing/re-gate
		//   jobs or cancel remaining jobs when healing is not configured or exhausted).
		// - If it is a non-gate job (mod/heal), cancel remaining non-terminal jobs so the run
		//   can reach a terminal state instead of leaving jobs stranded.
		if jobStatus == store.JobStatusFailed {
			jobs, jobsErr := st.ListJobsByRun(r.Context(), req.RunID.String())
			if jobsErr != nil {
				slog.Error("complete job: failed to list jobs for failure handling",
					"run_id", req.RunID,
					"job_id", req.JobID,
					"err", jobsErr,
				)
			} else {
				modType := strings.TrimSpace(job.ModType)
				if modType == "pre_gate" || modType == "post_gate" || modType == "re_gate" {
					if err := maybeCreateHealingJobs(r.Context(), st, run, req.RunID.String(), domaintypes.StepIndex(job.StepIndex), jobs); err != nil {
						slog.Error("complete job: failed to create healing jobs",
							"run_id", req.RunID,
							"job_id", req.JobID,
							"step_index", job.StepIndex,
							"err", err,
						)
					}
				} else {
					if err := cancelRemainingJobsAfterFailure(r.Context(), st, req.RunID.String(), domaintypes.StepIndex(job.StepIndex), jobs); err != nil {
						slog.Error("complete job: failed to cancel remaining jobs after non-gate failure",
							"run_id", req.RunID,
							"job_id", req.JobID,
							"step_index", job.StepIndex,
							"err", err,
						)
					}
				}
			}
		}

		// Server-driven scheduling: after job succeeds or is skipped, schedule the next job.
		// This transitions the first 'created' job to 'pending' so it can be claimed.
		if jobStatus == store.JobStatusSucceeded || jobStatus == store.JobStatusSkipped {
			// E4: Winner selection — When a re-gate succeeds, it's a "winner" branch.
			// Cancel all other parallel branch jobs (loser teardown) so only the winner proceeds.
			modType := strings.TrimSpace(job.ModType)
			if modType == "re_gate" {
				jobs, jobsErr := st.ListJobsByRun(r.Context(), req.RunID.String())
				if jobsErr != nil {
					slog.Error("complete job: failed to list jobs for winner selection",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"err", jobsErr,
					)
				} else {
					if err := cancelLoserBranches(r.Context(), st, req.RunID.String(), job, jobs); err != nil {
						slog.Error("complete job: failed to cancel loser branches",
							"run_id", req.RunID,
							"job_id", req.JobID,
							"step_index", job.StepIndex,
							"err", err,
						)
					}
				}
			}

			if _, err := st.ScheduleNextJob(r.Context(), req.RunID.String()); err != nil {
				// Log error but don't fail the job completion (job is already marked complete).
				// pgx.ErrNoRows means no more jobs to schedule, which is expected for the last job.
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to schedule next job",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"step_index", job.StepIndex,
						"err", err,
					)
				}
			}
		}

		// After completing a job, check if the run should transition to terminal state.
		// Derive the run's terminal status from the collective state of all jobs
		// instead of trusting the caller's status field.
		if err := maybeCompleteMultiStepRun(r.Context(), st, eventsService, run, req.RunID.String()); err != nil {
			// Log error but don't fail the job completion (job is already marked complete).
			slog.Error("complete job: failed to check run completion", "run_id", req.RunID, "job_id", req.JobID, "step_index", job.StepIndex, "err", err)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
