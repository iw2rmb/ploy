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

		// Decode request body to get run_id, status, reason, and stats.
		var req struct {
			RunID  string          `json:"run_id"`
			Status string          `json:"status"`
			Reason *string         `json:"reason,omitempty"`
			Stats  json.RawMessage `json:"stats,omitempty"`
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

		// Verify the run is in 'running' status before transitioning to terminal state.
		if run.Status != store.RunStatusRunning {
			http.Error(w, fmt.Sprintf("run status is %s, expected running", run.Status), http.StatusConflict)
			return
		}

		// Prepare stats field (default to empty JSON object if not provided).
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

		// Update stage status to terminal and set finished_at/duration.
		if stages, err := st.ListStagesByRun(r.Context(), runID); err == nil && len(stages) > 0 {
			now := time.Now().UTC()
			var stStatus store.StageStatus
			switch normalizedStatus {
			case store.RunStatusSucceeded:
				stStatus = store.StageStatusSucceeded
			case store.RunStatusFailed:
				stStatus = store.StageStatusFailed
			case store.RunStatusCanceled:
				stStatus = store.StageStatusCanceled
			default:
				stStatus = store.StageStatusFailed
			}
			dur := int64(0)
			if stages[0].StartedAt.Valid {
				d := now.Sub(stages[0].StartedAt.Time).Milliseconds()
				if d > 0 {
					dur = d
				}
			}
			_ = st.UpdateStageStatus(r.Context(), store.UpdateStageStatusParams{
				ID:         stages[0].ID,
				Status:     stStatus,
				StartedAt:  stages[0].StartedAt,
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
