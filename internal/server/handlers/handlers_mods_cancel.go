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
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// cancelTicketHandler cancels a Mods ticket (run) and transitions it to a terminal state.
// POST /v1/mods/{id}/cancel — Optional JSON body { reason?: string }
// Responses:
//   - 202 Accepted on state transition
//   - 200 OK if already terminal (idempotent)
//   - 404 Not Found if ticket does not exist
//   - 400 Bad Request for invalid id
func cancelTicketHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ticketIDStr := r.PathValue("id")
		if strings.TrimSpace(ticketIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse UUID
		pgID := domaintypes.ToPGUUID(ticketIDStr)
		if !pgID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Optional body: { reason?: string }
		var req struct {
			Reason *string `json:"reason"`
		}
		// Empty body is allowed; decode only if body has data
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Load current run
		run, err := st.GetRun(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "ticket not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get ticket: %v", err), http.StatusInternalServerError)
			slog.Error("cancel ticket: lookup failed", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// If already terminal, idempotent 200 OK
		if run.Status == store.RunStatusSucceeded || run.Status == store.RunStatusFailed || run.Status == store.RunStatusCanceled {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Transition to canceled; set finished_at to now.
		now := time.Now().UTC()
		err = st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{
			ID:         pgID,
			Status:     store.RunStatusCanceled,
			FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to cancel ticket: %v", err), http.StatusInternalServerError)
			slog.Error("cancel ticket: update run failed", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// Best-effort job updates to canceled — only for created|scheduled|running jobs
		if jobs, err := st.ListJobsByRun(r.Context(), pgID); err == nil && len(jobs) > 0 {
			for _, job := range jobs {
				if job.Status != store.JobStatusCreated && job.Status != store.JobStatusScheduled && job.Status != store.JobStatusRunning {
					continue
				}
				// Compute duration if started
				dur := int64(0)
				if job.StartedAt.Valid {
					d := now.Sub(job.StartedAt.Time).Milliseconds()
					if d > 0 {
						dur = d
					}
				}
				_ = st.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
					ID:         job.ID,
					Status:     store.JobStatusCanceled,
					StartedAt:  job.StartedAt,
					FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
					DurationMs: dur,
				})
			}
		}

		// Publish terminal ticket event + done status for SSE clients
		if eventsService != nil {
			ticketSummary := modsapi.TicketSummary{
				TicketID:   domaintypes.TicketID(uuid.UUID(pgID.Bytes).String()),
				State:      modsapi.TicketStateCancelled,
				Repository: run.RepoUrl,
				CreatedAt:  timeOrZero(run.CreatedAt),
				UpdatedAt:  now,
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if req.Reason != nil && strings.TrimSpace(*req.Reason) != "" {
				if ticketSummary.Metadata == nil {
					ticketSummary.Metadata = map[string]string{}
				}
				ticketSummary.Metadata["reason"] = strings.TrimSpace(*req.Reason)
			}
			if err := eventsService.PublishTicket(r.Context(), uuid.UUID(pgID.Bytes).String(), ticketSummary); err != nil {
				slog.Error("cancel ticket: publish ticket event failed", "ticket_id", ticketIDStr, "err", err)
			}
			// Signal done on the stream
			if err := eventsService.Hub().PublishStatus(r.Context(), uuid.UUID(pgID.Bytes).String(), logstream.Status{Status: "done"}); err != nil {
				slog.Error("cancel ticket: publish done status failed", "ticket_id", ticketIDStr, "err", err)
			}
		}

		w.WriteHeader(http.StatusAccepted)
		slog.Info("ticket canceled", "ticket_id", ticketIDStr, "had_reason", req.Reason != nil)
	}
}
