package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ackRunStartHandler acknowledges that a node has started executing a run.
// Transitions run status from 'assigned' to 'running'.
func ackRunStartHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
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

		// Decode request body to get run_id and optional step_index.
		// For multi-step runs, nodeagent includes step_index to trigger step-level ack.
		var req struct {
			RunID     string `json:"run_id"`
			StepIndex *int32 `json:"step_index,omitempty"` // Present for step-level claims
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

		var err error
		// Verify node exists before attempting to acknowledge.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("ack run start: node check failed", "node_id", nodeIDStr, "err", err)
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
			slog.Error("ack run start: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// If step_index is present, this is a step-level ack (multi-step run).
		// Otherwise, it's a run-level ack (single-step or legacy run).
		if req.StepIndex != nil {
			// Step-level ack: retrieve the run_step and transition it from assigned→running.
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
				slog.Error("ack run step start: get step failed", "run_id", req.RunID, "step_index", *req.StepIndex, "err", err)
				return
			}

			// Verify the step is assigned to the requesting node.
			if !runStep.NodeID.Valid || runStep.NodeID != nodeID {
				http.Error(w, "run step not assigned to this node", http.StatusForbidden)
				return
			}

			// Verify the step is in 'assigned' status before transitioning to 'running'.
			if runStep.Status != store.RunStepStatusAssigned {
				http.Error(w, fmt.Sprintf("run step status is %s, expected assigned", runStep.Status), http.StatusConflict)
				return
			}

			// Transition run_step status from 'assigned' to 'running'.
			err = st.AckRunStepStart(r.Context(), runStep.ID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to acknowledge run step start: %v", err), http.StatusInternalServerError)
				slog.Error("ack run step start: update failed", "run_id", req.RunID, "step_index", *req.StepIndex, "node_id", nodeIDStr, "err", err)
				return
			}

			// For multi-step runs, transition the run status to 'running' when the first step starts.
			// This ensures run.status reflects that execution has begun, even though individual steps
			// are claimed via run_steps. We relax the status precondition to allow queued→running transition.
			// AckRunStart is idempotent and only updates if status is 'assigned', so for subsequent steps
			// (where run.status is already 'running'), this is a no-op.
			//
			// Note: The run may be in 'queued' status (no node_id) for multi-step runs since steps are
			// claimed independently. We call AckRunStart regardless; if run.status is already 'running'
			// or not 'assigned', the query will not match and that's acceptable (silent no-op).
			_ = st.AckRunStart(r.Context(), runID)

			slog.Info("run step start acknowledged",
				"run_id", req.RunID,
				"step_index", *req.StepIndex,
				"node_id", nodeIDStr,
				"status", "running",
			)
		} else {
			// Run-level ack: verify the run is assigned to this node and transition from assigned→running.
			// This path is used for single-step runs or legacy runs without run_steps rows.
			if !run.NodeID.Valid || run.NodeID != nodeID {
				http.Error(w, "run not assigned to this node", http.StatusForbidden)
				return
			}

			// Verify the run is in 'assigned' status before transitioning to 'running'.
			if run.Status != store.RunStatusAssigned {
				http.Error(w, fmt.Sprintf("run status is %s, expected assigned", run.Status), http.StatusConflict)
				return
			}

			// Transition run status from 'assigned' to 'running'.
			err = st.AckRunStart(r.Context(), runID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to acknowledge run start: %v", err), http.StatusInternalServerError)
				slog.Error("ack run start: update failed", "run_id", req.RunID, "node_id", nodeIDStr, "err", err)
				return
			}

			slog.Info("run start acknowledged",
				"run_id", req.RunID,
				"node_id", nodeIDStr,
				"status", "running",
			)
		}

		// Update stage to running and set started_at.
		if stages, err := st.ListStagesByRun(r.Context(), runID); err == nil && len(stages) > 0 {
			_ = st.UpdateStageStatus(r.Context(), store.UpdateStageStatusParams{
				ID:         stages[0].ID,
				Status:     store.StageStatusRunning,
				StartedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				FinishedAt: pgtype.Timestamptz{},
				DurationMs: 0,
			})
		}

		// Publish running event to SSE hub.
		if eventsService != nil {
			ticketSummary := modsapi.TicketSummary{
				TicketID:   domaintypes.TicketID(req.RunID),
				State:      modsapi.TicketStateRunning,
				Repository: run.RepoUrl,
				CreatedAt:  run.CreatedAt.Time,
				UpdatedAt:  time.Now().UTC(),
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishTicket(r.Context(), req.RunID, ticketSummary); err != nil {
				slog.Error("ack run start: publish ticket event failed", "ticket_id", req.RunID, "err", err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
