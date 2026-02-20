package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// parseLastEventID parses the Last-Event-ID header to support SSE resumption.
// Returns 0 if the header is absent, invalid, or negative.
// Uses types.EventID for type-safe cursor handling.
func parseLastEventID(header string) domaintypes.EventID {
	if header == "" {
		return 0
	}
	var eid domaintypes.EventID
	if err := eid.UnmarshalText([]byte(header)); err != nil {
		return 0
	}
	if !eid.Valid() {
		return 0
	}
	return eid
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
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify run exists in the database.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				httpErr(w, http.StatusNotFound, "run not found")
			default:
				slog.Error("get run logs: database error", "run_id", runID.String(), "err", err)
				httpErr(w, http.StatusInternalServerError, "failed to get run")
			}
			return
		}

		// Parse Last-Event-ID header for resumption support.
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// Get the hub from the events service.
		hub := eventsService.Hub()

		// Ensure the stream exists (creates if not present).
		// Validation happens inside Ensure; errors are logged but stream proceeds.
		if err := hub.Ensure(runID); err != nil {
			slog.Error("ensure stream failed", "run_id", runID.String(), "err", err)
			httpErr(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// Delegate to logstream.Serve for SSE streaming.
		if err := logstream.Serve(w, r, hub, runID, sinceID); err != nil {
			// Only log non-cancellation errors (client disconnect is normal).
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run logs", "run_id", runID.String(), "err", err)
			}
		}
	}
}
