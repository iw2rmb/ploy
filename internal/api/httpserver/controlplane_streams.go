package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func modsLogStreamID(ticketID string) string {
	return strings.TrimSpace(ticketID)
}

func (s *controlPlaneServer) snapshotLogStream(ctx context.Context, streamID string) ([]logstream.Event, error) {
	if events := s.streams.Snapshot(streamID); len(events) > 0 {
		return events, nil
	}
	sub, err := s.streams.Subscribe(ctx, streamID, 0)
	if err != nil {
		return nil, err
	}
	defer sub.Cancel()
	events := make([]logstream.Event, 0, 8)
	for {
		select {
		case evt, ok := <-sub.Events:
			if !ok {
				return events, nil
			}
			events = append(events, evt)
			if strings.EqualFold(evt.Type, "done") {
				return events, nil
			}
		default:
			return events, nil
		}
	}
}

func buildLogEventDTOs(events []logstream.Event) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, evt := range events {
		dto := map[string]any{
			"id":   evt.ID,
			"type": evt.Type,
		}
		if len(evt.Data) > 0 {
			var payload any
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				dto["data"] = strings.TrimSpace(string(evt.Data))
			} else {
				dto["data"] = payload
			}
		}
		out = append(out, dto)
	}
	return out
}

func writeSSEJSON(w io.Writer, id int64, event string, payload any) error {
	if id > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", id); err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(w, "\n")
	return err
}

func parseLastEventID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if id < 0 {
		return 0, nil
	}
	return id, nil
}
