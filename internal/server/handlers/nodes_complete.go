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
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
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

		// Parse and validate node_id.
		nodeID := domaintypes.ToPGUUID(nodeIDStr)
		if !nodeID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id, status, reason, stats, and optional step_index.
		// For multi-step runs, nodeagent includes step_index to trigger step-level completion.
		var req struct {
			RunID     string          `json:"run_id"`
			Status    string          `json:"status"`
			Reason    *string         `json:"reason,omitempty"`
			Stats     json.RawMessage `json:"stats,omitempty"`
			StepIndex *int32          `json:"step_index,omitempty"` // Present for step-level completions
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runID := domaintypes.ToPGUUID(req.RunID)
		if !runID.Valid {
			http.Error(w, "invalid run_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Validate and convert status to canonical RunStatus type.
		// This uses ConvertToRunStatus to handle various API representations
		// (e.g., "cancelled" vs "canceled", "pending" -> "queued").
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
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists and is assigned to this node.
		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Verify the run is assigned to the requesting node.
		if !run.NodeID.Valid || run.NodeID != nodeID {
			http.Error(w, "run not assigned to this node", http.StatusForbidden)
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

		// If step_index is present, this is a step-level completion (multi-step run).
		// Otherwise, it's a run-level completion (single-step or legacy run).
		if req.StepIndex != nil {
			// Step-level completion: retrieve the run_step and transition it to terminal state.
			runStep, err := st.GetRunStepByIndex(r.Context(), store.GetRunStepByIndexParams{
				RunID:     runID,
				StepIndex: *req.StepIndex,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "run step not found", http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed to get run step: %v", err), http.StatusInternalServerError)
				slog.Error("complete run step: get step failed", "run_id", req.RunID, "step_index", *req.StepIndex, "err", err)
				return
			}

			// Verify the step is assigned to the requesting node.
			if !runStep.NodeID.Valid || runStep.NodeID != nodeID {
				http.Error(w, "run step not assigned to this node", http.StatusForbidden)
				return
			}

			// Verify the step is in 'running' status before transitioning to terminal state.
			if runStep.Status != store.RunStepStatusRunning {
				http.Error(w, fmt.Sprintf("run step status is %s, expected running", runStep.Status), http.StatusConflict)
				return
			}

			// Map run terminal status (succeeded/failed/canceled) to RunStepStatus.
			var stepStatus store.RunStepStatus
			switch normalizedStatus {
			case store.RunStatusSucceeded:
				stepStatus = store.RunStepStatusSucceeded
			case store.RunStatusFailed:
				stepStatus = store.RunStepStatusFailed
			case store.RunStatusCanceled:
				stepStatus = store.RunStepStatusCanceled
			default:
				// Fallback for unexpected terminal states.
				stepStatus = store.RunStepStatusFailed
			}

			// Transition run_step status to terminal state (succeeded/failed/canceled).
			// Sets finished_at timestamp and optional reason for failure/cancellation.
			err = st.UpdateRunStepCompletion(r.Context(), store.UpdateRunStepCompletionParams{
				ID:     runStep.ID,
				Status: stepStatus,
				Reason: req.Reason,
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to complete run step: %v", err), http.StatusInternalServerError)
				slog.Error("complete run step: update failed", "run_id", req.RunID, "step_index", *req.StepIndex, "node_id", nodeIDStr, "err", err)
				return
			}

			slog.Info("run step completed",
				"run_id", req.RunID,
				"step_index", *req.StepIndex,
				"node_id", nodeIDStr,
				"status", stepStatus,
				"has_reason", req.Reason != nil,
				"stats_size", len(statsBytes),
			)

			// After completing a step, check if the run should transition to terminal state.
			// Derive the run's terminal status from the collective state of all run_steps
			// instead of trusting the caller's status field.
			if err := maybeCompleteMultiStepRun(r.Context(), st, eventsService, run, runID); err != nil {
				// Log error but don't fail the step completion (step is already marked complete).
				slog.Error("complete run step: failed to check run completion", "run_id", req.RunID, "step_index", *req.StepIndex, "err", err)
			}

			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Run-level completion: verify the run is in 'running' status before transitioning to terminal state.
		// This path is used for single-step runs or legacy runs without run_steps rows.
		if run.Status != store.RunStatusRunning {
			http.Error(w, fmt.Sprintf("run status is %s, expected running", run.Status), http.StatusConflict)
			return
		}

		// Update run completion: set status, reason, finished_at (server-side now()), and stats.
		err = st.UpdateRunCompletion(r.Context(), store.UpdateRunCompletionParams{
			ID:     runID,
			Status: normalizedStatus,
			Reason: req.Reason,
			Stats:  statsBytes,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete run: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: update failed", "run_id", req.RunID, "node_id", nodeIDStr, "err", err)
			return
		}

		// Update job status to terminal and set finished_at/duration.
		if jobs, err := st.ListJobsByRun(r.Context(), runID); err == nil && len(jobs) > 0 {
			now := time.Now().UTC()
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
			dur := int64(0)
			if jobs[0].StartedAt.Valid {
				d := now.Sub(jobs[0].StartedAt.Time).Milliseconds()
				if d > 0 {
					dur = d
				}
			}
			_ = st.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
				ID:         jobs[0].ID,
				Status:     jobStatus,
				StartedAt:  jobs[0].StartedAt,
				FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
				DurationMs: dur,
			})
		}

		// Publish terminal ticket event and done status to SSE hub.
		if eventsService != nil {
			// Map store.RunStatus to modsapi.TicketState.
			var ticketState modsapi.TicketState
			switch normalizedStatus {
			case store.RunStatusSucceeded:
				ticketState = modsapi.TicketStateSucceeded
			case store.RunStatusFailed:
				ticketState = modsapi.TicketStateFailed
			case store.RunStatusCanceled:
				ticketState = modsapi.TicketStateCancelled
			default:
				ticketState = modsapi.TicketStateFailed
			}

			ticketSummary := modsapi.TicketSummary{
				TicketID:   domaintypes.TicketID(req.RunID),
				State:      ticketState,
				Repository: run.RepoUrl,
				CreatedAt:  run.CreatedAt.Time,
				UpdatedAt:  time.Now().UTC(),
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishTicket(r.Context(), req.RunID, ticketSummary); err != nil {
				slog.Error("complete run: publish ticket event failed", "ticket_id", req.RunID, "err", err)
			}

			// Publish done event to signal stream completion.
			doneStatus := logstream.Status{Status: "done"}
			if err := eventsService.Hub().PublishStatus(r.Context(), req.RunID, doneStatus); err != nil {
				slog.Error("complete run: publish done status failed", "run_id", req.RunID, "err", err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run completed",
			"run_id", req.RunID,
			"node_id", nodeIDStr,
			"status", req.Status,
			"has_reason", req.Reason != nil,
			"stats_size", len(statsBytes),
		)
	}
}

// maybeCompleteMultiStepRun checks if all steps of a multi-step run are complete
// and transitions the run to its terminal state (succeeded/failed/canceled).
// This function derives the run's terminal status from the collective state of
// all run_steps instead of trusting the caller's status field.
//
// Status derivation rules:
// - If any step failed, the run is marked as failed.
// - If any step was canceled, the run is marked as canceled (unless a step failed).
// - If all steps succeeded, the run is marked as succeeded.
// - If steps are still pending/assigned/running, the run remains in running state.
func maybeCompleteMultiStepRun(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID pgtype.UUID) error {
	// Count the total number of steps for this run.
	totalSteps, err := st.CountRunSteps(ctx, runID)
	if err != nil {
		return fmt.Errorf("count run steps: %w", err)
	}

	// If there are no run_steps, this is a legacy single-step run (not a multi-step run).
	// The caller path (run-level completion) already handles this case.
	if totalSteps == 0 {
		return nil
	}

	// Count steps by terminal status to determine the run's effective state.
	succeededCount, err := st.CountRunStepsByStatus(ctx, store.CountRunStepsByStatusParams{
		RunID:  runID,
		Status: store.RunStepStatusSucceeded,
	})
	if err != nil {
		return fmt.Errorf("count succeeded steps: %w", err)
	}

	failedCount, err := st.CountRunStepsByStatus(ctx, store.CountRunStepsByStatusParams{
		RunID:  runID,
		Status: store.RunStepStatusFailed,
	})
	if err != nil {
		return fmt.Errorf("count failed steps: %w", err)
	}

	canceledCount, err := st.CountRunStepsByStatus(ctx, store.CountRunStepsByStatusParams{
		RunID:  runID,
		Status: store.RunStepStatusCanceled,
	})
	if err != nil {
		return fmt.Errorf("count canceled steps: %w", err)
	}

	// Calculate terminal steps (succeeded + failed + canceled).
	terminalSteps := succeededCount + failedCount + canceledCount

	// If not all steps are in terminal state, the run is still in progress.
	// Do not transition the run to a terminal state yet.
	if terminalSteps < totalSteps {
		slog.Debug("multi-step run still in progress",
			"run_id", runID,
			"total_steps", totalSteps,
			"terminal_steps", terminalSteps,
			"succeeded", succeededCount,
			"failed", failedCount,
			"canceled", canceledCount,
		)
		return nil
	}

	// All steps are in terminal state. Derive the run's terminal status.
	// Priority: failed > canceled > succeeded.
	var runStatus store.RunStatus
	var reason *string

	if failedCount > 0 {
		// At least one step failed: mark the run as failed.
		runStatus = store.RunStatusFailed
		reasonMsg := fmt.Sprintf("%d of %d steps failed", failedCount, totalSteps)
		reason = &reasonMsg
	} else if canceledCount > 0 {
		// At least one step was canceled (and no failures): mark the run as canceled.
		runStatus = store.RunStatusCanceled
		reasonMsg := fmt.Sprintf("%d of %d steps canceled", canceledCount, totalSteps)
		reason = &reasonMsg
	} else {
		// All steps succeeded: mark the run as succeeded.
		runStatus = store.RunStatusSucceeded
	}

	slog.Info("multi-step run completing",
		"run_id", runID,
		"total_steps", totalSteps,
		"succeeded", succeededCount,
		"failed", failedCount,
		"canceled", canceledCount,
		"derived_status", runStatus,
	)

	// Transition the run to its terminal status.
	// Use empty JSON object for stats (step-level stats are tracked per step).
	err = st.UpdateRunCompletion(ctx, store.UpdateRunCompletionParams{
		ID:     runID,
		Status: runStatus,
		Reason: reason,
		Stats:  []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("update run completion: %w", err)
	}

	// Update job status to terminal and set finished_at/duration.
	if jobs, err := st.ListJobsByRun(ctx, runID); err == nil && len(jobs) > 0 {
		now := time.Now().UTC()
		var jobStatus store.JobStatus
		switch runStatus {
		case store.RunStatusSucceeded:
			jobStatus = store.JobStatusSucceeded
		case store.RunStatusFailed:
			jobStatus = store.JobStatusFailed
		case store.RunStatusCanceled:
			jobStatus = store.JobStatusCanceled
		default:
			jobStatus = store.JobStatusFailed
		}
		dur := int64(0)
		if jobs[0].StartedAt.Valid {
			d := now.Sub(jobs[0].StartedAt.Time).Milliseconds()
			if d > 0 {
				dur = d
			}
		}
		_ = st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         jobs[0].ID,
			Status:     jobStatus,
			StartedAt:  jobs[0].StartedAt,
			FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
			DurationMs: dur,
		})
	}

	// Publish terminal ticket event and done status to SSE hub.
	if eventsService != nil {
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
			slog.Error("complete multi-step run: publish ticket event failed", "run_id", runID, "err", err)
		}

		// Publish done event to signal stream completion.
		doneStatus := logstream.Status{Status: "done"}
		if err := eventsService.Hub().PublishStatus(ctx, runUUID.String(), doneStatus); err != nil {
			slog.Error("complete multi-step run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("multi-step run completed",
		"run_id", runID,
		"status", runStatus,
		"has_reason", reason != nil,
	)

	return nil
}
