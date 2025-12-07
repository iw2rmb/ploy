package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

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
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
// Validation is limited to non-empty check; the database layer rejects invalid IDs.
func getModEventsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract ticket ID from path parameter.
		// Run IDs are KSUID strings (27 chars); treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Verify run exists in the database using string ID directly.
		// No UUID parsing needed; store accepts KSUID strings.
		_, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "ticket not found", http.StatusNotFound)
				return
			}
			slog.Error("get mod events: database error", "ticket_id", runIDStr, "err", err)
			http.Error(w, "failed to get ticket", http.StatusInternalServerError)
			return
		}

		// Parse Last-Event-ID header for resumption support.
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// Get the hub from the events service.
		hub := eventsService.Hub()

		// Ensure the stream exists (creates if not present).
		hub.Ensure(runIDStr)

		// Delegate to logstream.Serve for SSE streaming.
		if err := logstream.Serve(w, r, hub, runIDStr, sinceID); err != nil {
			// Only log non-cancellation errors (client disconnect is normal).
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream mod events", "ticket_id", runIDStr, "err", err)
			}
		}
	}
}
