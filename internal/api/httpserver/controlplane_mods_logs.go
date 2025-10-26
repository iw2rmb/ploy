package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// handleModsLogsSnapshot returns the buffered stream events for a ticket.
func (s *controlPlaneServer) handleModsLogsSnapshot(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	events, err := s.snapshotLogStream(r.Context(), modsLogStreamID(ticketID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	payload := map[string]any{
		"events": buildLogEventDTOs(events),
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleModsLogsStream proxies live log streams for a ticket via Server-Sent Events.
func (s *controlPlaneServer) handleModsLogsStream(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if err := logstream.Serve(w, r, s.streams, modsLogStreamID(ticketID), since); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}
