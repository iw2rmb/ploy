package httpserver

import (
	"io"
	"net/http"
	"time"

	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
)

// handleModsEvents streams MODS ticket and stage updates via SSE.
func (s *controlPlaneServer) handleModsEvents(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mods == nil {
		http.Error(w, "mods service unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}

	status, rev, err := s.mods.TicketStatusWithRevision(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if rev > 0 && (since <= 0 || rev > since) {
		if err := writeSSEJSON(w, rev, "ticket", toAPITicketSummary(status)); err != nil {
			return
		}
		flusher.Flush()
		since = rev
	}

	events, err := s.mods.WatchTicket(r.Context(), ticketID, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			switch evt.Kind {
			case controlplanemods.EventTicket:
				if evt.Ticket == nil {
					continue
				}
				if err := writeSSEJSON(w, evt.Revision, "ticket", toAPITicketSummary(evt.Ticket)); err != nil {
					return
				}
				flusher.Flush()
			case controlplanemods.EventStage:
				if evt.Stage == nil {
					continue
				}
				payload := modsStageEvent{
					TicketID: ticketID,
					Stage:    toAPIStageStatus(evt.Stage),
				}
				if err := writeSSEJSON(w, evt.Revision, "stage", payload); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ":ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
