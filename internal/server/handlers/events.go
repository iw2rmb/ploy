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

// getRunLogsHandler returns an HTTP handler that streams run logs and events over SSE.
// Supports Last-Event-ID header for resuming streams from a specific event.
// GET /v1/runs/{id}/logs — Native SSE for run logs/events.
//
// Run IDs are now KSUID-backed strings (27 characters). We perform a cheap
// length check to reject obviously invalid IDs before hitting the store; the
// database layer enforces existence.
func getRunLogsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run ID from path parameter.
		// Run IDs are KSUID strings (27 chars); treated as opaque identifiers.
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Reject obviously invalid IDs to avoid hanging SSE streams on garbage.
		if len(runIDStr) != 27 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		// Verify run exists in the database using string ID directly.
		// No UUID parsing needed; store accepts KSUID strings.
		_, err = st.GetRun(r.Context(), runIDStr)
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				http.Error(w, "run not found", http.StatusNotFound)
			default:
				slog.Error("get run logs: database error", "run_id", runIDStr, "err", err)
				http.Error(w, "failed to get run", http.StatusInternalServerError)
			}
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
				slog.Error("stream run logs", "run_id", runIDStr, "err", err)
			}
		}
	}
}
