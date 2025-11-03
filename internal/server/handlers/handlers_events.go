package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// parseLastEventID parses the Last-Event-ID header to support SSE resumption.
// Returns 0 if the header is absent or invalid.
func parseLastEventID(header string) int64 {
	if header == "" {
		return 0
	}
	id, err := strconv.ParseInt(strings.TrimSpace(header), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// getModEventsHandler returns an HTTP handler that streams mod (ticket) events over SSE.
// Supports Last-Event-ID header for resuming streams from a specific event.
// GET /v1/mods/{id}/events — Native SSE under mods (no proxy).
func getModEventsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract ticket ID from path parameter.
		ticketIDStr := r.PathValue("id")
		if strings.TrimSpace(ticketIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate ticket_id.
		ticketUUID, err := uuid.Parse(ticketIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Verify run exists in the database (ticket_id == run UUID).
		_, err = st.GetRun(r.Context(), pgtype.UUID{
			Bytes: ticketUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "ticket not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get ticket: %v", err), http.StatusInternalServerError)
			slog.Error("get mod events: database error", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// Parse Last-Event-ID header for resumption support.
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// Get the hub from the events service.
		hub := eventsService.Hub()

		// Ensure the stream exists (creates if not present).
		hub.Ensure(ticketIDStr)

		// Delegate to logstream.Serve for SSE streaming.
		if err := logstream.Serve(w, r, hub, ticketIDStr, sinceID); err != nil {
			// Only log non-cancellation errors (client disconnect is normal).
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream mod events", "ticket_id", ticketIDStr, "err", err)
			}
		}
	}
}
