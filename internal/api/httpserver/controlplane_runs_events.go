package httpserver

import (
    "errors"
    "io"
    "net/http"
    "time"

    "github.com/jackc/pgx/v5"

    "github.com/iw2rmb/ploy/internal/store"
)

// handleRunsEvents implements Server-Sent Events (SSE) for run events.
// Endpoint: GET /v1/runs/{id}/events
// This provides basic log/event fanout for a run.
func (s *controlPlaneServer) handleRunsEvents(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if streaming is supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Parse run ID
	runUUID, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}

	// Verify run exists
	_, err = s.store.GetRun(r.Context(), runUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Parse Last-Event-ID for reconnection support
	sinceID, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Send initial sync marker
	io.WriteString(w, ":ok\n\n")
	flusher.Flush()

	// Get initial events (or events since last ID)
	var events []EventDTO
	if sinceID > 0 {
		storeEvents, err := s.store.ListEventsByRunSince(r.Context(), store.ListEventsByRunSinceParams{
			RunID: runUUID,
			ID:    sinceID,
		})
		if err != nil {
			// Log error but don't fail the stream
			io.WriteString(w, ":error fetching events\n\n")
			flusher.Flush()
		} else {
			for _, evt := range storeEvents {
				events = append(events, eventDTOFrom(evt))
			}
		}
	} else {
		storeEvents, err := s.store.ListEventsByRun(r.Context(), runUUID)
		if err != nil {
			io.WriteString(w, ":error fetching events\n\n")
			flusher.Flush()
		} else {
			for _, evt := range storeEvents {
				events = append(events, eventDTOFrom(evt))
			}
		}
	}

	// Send initial events
	for _, evt := range events {
		if err := writeSSEJSON(w, evt.ID, "event", evt); err != nil {
			return
		}
		flusher.Flush()
	}

	// Start polling for new events (basic implementation)
	// In a production system, this could use database LISTEN/NOTIFY or a message queue
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	lastEventID := sinceID
	if len(events) > 0 {
		lastEventID = events[len(events)-1].ID
	}

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Poll for new events
			newEvents, err := s.store.ListEventsByRunSince(r.Context(), store.ListEventsByRunSinceParams{
				RunID: runUUID,
				ID:    lastEventID,
			})
			if err != nil {
				continue
			}
			for _, evt := range newEvents {
				dto := eventDTOFrom(evt)
				if err := writeSSEJSON(w, dto.ID, "event", dto); err != nil {
					return
				}
				flusher.Flush()
				lastEventID = dto.ID
			}
		case <-heartbeat.C:
			// Send periodic heartbeat to keep connection alive
			io.WriteString(w, ":ping\n\n")
			flusher.Flush()
		}
	}
}
