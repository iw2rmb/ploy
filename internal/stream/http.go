package logstream

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var errNoHub = errors.New("logstream: hub unavailable")

// Serve streams events for the provided stream over SSE.
func Serve(w http.ResponseWriter, r *http.Request, hub *Hub, streamID string, sinceID int64) error {
	if hub == nil {
		return errNoHub
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("logstream: response does not support streaming")
	}

	hub.Ensure(streamID)
	sub, err := hub.Subscribe(r.Context(), streamID, sinceID)
	if err != nil {
		return err
	}
	defer sub.Cancel()

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return err
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if err := writeEventFrame(w, evt); err != nil {
				return err
			}
			flusher.Flush()
			if strings.EqualFold(evt.Type, "done") {
				return nil
			}
		}
	}
}

// ServeFiltered streams events for the provided stream over SSE, applying an optional
// filter/transform function before writing frames.
//
// If filter returns ok=false, the event is skipped.
func ServeFiltered(w http.ResponseWriter, r *http.Request, hub *Hub, streamID string, sinceID int64, filter func(Event) (Event, bool)) error {
	if hub == nil {
		return errNoHub
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("logstream: response does not support streaming")
	}

	hub.Ensure(streamID)
	sub, err := hub.Subscribe(r.Context(), streamID, sinceID)
	if err != nil {
		return err
	}
	defer sub.Cancel()

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return err
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if filter != nil {
				var keep bool
				evt, keep = filter(evt)
				if !keep {
					continue
				}
			}
			if err := writeEventFrame(w, evt); err != nil {
				return err
			}
			flusher.Flush()
			if strings.EqualFold(evt.Type, "done") {
				return nil
			}
		}
	}
}

func writeEventFrame(w io.Writer, evt Event) error {
	if evt.ID > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", evt.ID); err != nil {
			return err
		}
	}
	if evt.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", evt.Type); err != nil {
			return err
		}
	}
	if len(evt.Data) > 0 {
		lines := strings.Split(string(evt.Data), "\n")
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(w, "data:"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}
