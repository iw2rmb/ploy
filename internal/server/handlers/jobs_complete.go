package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
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
		jobIDStr := r.PathValue("job_id")
		if strings.TrimSpace(jobIDStr) == "" {
			http.Error(w, "job_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate job_id as UUID.
		jobID := domaintypes.ToPGUUID(jobIDStr)
		if !jobID.Valid {
			http.Error(w, "invalid job_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Extract node identity from mTLS certificate.
		// The CommonName contains the node UUID assigned during PKI bootstrap.
		identity, ok := auth.IdentityFromContext(ctx)
		if !ok {
			http.Error(w, "unauthorized: no identity in context", http.StatusUnauthorized)
			return
		}
		nodeID := domaintypes.ToPGUUID(identity.CommonName)
		if !nodeID.Valid {
			http.Error(w, "unauthorized: invalid node identity", http.StatusUnauthorized)
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

		// Look up the job by job_id.
		job, err := st.GetJob(ctx, jobID)
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

		// Verify the job is assigned to the calling node (authorization check).
		if !job.NodeID.Valid || job.NodeID != nodeID {
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
				"node_id", identity.CommonName,
				"err", err,
			)
			return
		}

		slog.Info("job completed",
			"job_id", jobIDStr,
			"step_index", job.StepIndex,
			"node_id", identity.CommonName,
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

		// If gate job failed, check if healing jobs should be created.
		if jobStatus == store.JobStatusFailed && err == nil {
			jobs, jobsErr := st.ListJobsByRun(ctx, runID)
			if jobsErr != nil {
				slog.Error("complete job: failed to list jobs for healing",
					"job_id", jobIDStr,
					"err", jobsErr,
				)
			} else if healErr := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(job.StepIndex), jobs); healErr != nil {
				slog.Error("complete job: failed to create healing jobs",
					"job_id", jobIDStr,
					"step_index", job.StepIndex,
					"err", healErr,
				)
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

// maybeCompleteMultiStepRunForJob is a helper that publishes terminal events
// when a run completes. It's used by the job-level completion endpoint.
// This reuses the existing maybeCompleteMultiStepRun logic from nodes_complete.go.
// Note: The actual implementation is in nodes_complete.go and is shared.

// publishRunCompletionEvents publishes terminal ticket and done status events.
// This is a helper extracted from maybeCompleteMultiStepRun for potential reuse.
func publishRunCompletionEvents(ctx context.Context, eventsService *events.Service, run store.Run, runID pgtype.UUID, runStatus store.RunStatus) {
	if eventsService == nil {
		return
	}

	// Map store.RunStatus to modsapi.TicketState.
	var ticketState modsapi.TicketState
	switch runStatus {
	case store.RunStatusSucceeded:
		ticketState = modsapi.TicketStateSucceeded
	case store.RunStatusFailed:
		ticketState = modsapi.TicketStateFailed
	case store.RunStatusCanceled:
		ticketState = modsapi.TicketStateCancelled
	default:
		ticketState = modsapi.TicketStateFailed
	}

	runUUID := uuid.UUID(runID.Bytes)
	ticketSummary := modsapi.TicketSummary{
		TicketID:   domaintypes.TicketID(runUUID.String()),
		State:      ticketState,
		Repository: run.RepoUrl,
		CreatedAt:  run.CreatedAt.Time,
		UpdatedAt:  time.Now().UTC(),
		Stages:     make(map[string]modsapi.StageStatus),
	}
	if err := eventsService.PublishTicket(ctx, runUUID.String(), ticketSummary); err != nil {
		slog.Error("publish run completion: ticket event failed", "run_id", runID, "err", err)
	}

	// Publish done event to signal stream completion.
	doneStatus := logstream.Status{Status: "done"}
	if err := eventsService.Hub().PublishStatus(ctx, runUUID.String(), doneStatus); err != nil {
		slog.Error("publish run completion: done status failed", "run_id", runID, "err", err)
	}
}
