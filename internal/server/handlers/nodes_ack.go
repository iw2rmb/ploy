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
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id.
		var req struct {
			RunID string `json:"run_id"`
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
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to acknowledge.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
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
		run, err := st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("ack run start: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Verify the run is assigned to the requesting node.
		if !run.NodeID.Valid || uuid.UUID(run.NodeID.Bytes) != nodeUUID {
			http.Error(w, "run not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the run is in 'assigned' status before transitioning to 'running'.
		if run.Status != store.RunStatusAssigned {
			http.Error(w, fmt.Sprintf("run status is %s, expected assigned", run.Status), http.StatusConflict)
			return
		}

		// Transition run status from 'assigned' to 'running'.
		err = st.AckRunStart(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to acknowledge run start: %v", err), http.StatusInternalServerError)
			slog.Error("ack run start: update failed", "run_id", req.RunID, "node_id", nodeIDStr, "err", err)
			return
		}

		// Update stage to running and set started_at.
		if stages, err := st.ListStagesByRun(r.Context(), pgtype.UUID{Bytes: runUUID, Valid: true}); err == nil && len(stages) > 0 {
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
		slog.Info("run start acknowledged",
			"run_id", req.RunID,
			"node_id", nodeIDStr,
			"status", "running",
		)
	}
}
