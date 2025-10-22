package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/node/httpapi"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

func TestNodeLogsStreamDeliversEvents(t *testing.T) {
	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 16})
	jobID := "job-node-1"
	streams.Ensure(jobID)

	server := httptest.NewServer(httpapi.New(streams))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := make(chan sseEvent, 4)
	errCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/node/v2/jobs/%s/logs/stream", server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			events <- evt
			if evt.Type == "done" {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := streams.PublishLog(context.Background(), jobID, logstream.LogRecord{Timestamp: "2025-10-22T13:00:00Z", Stream: "stdout", Line: "node-start"}); err != nil {
		t.Fatalf("publish log: %v", err)
	}
	if err := streams.PublishStatus(context.Background(), jobID, logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	expect := []string{"log", "done"}
	for _, typ := range expect {
		select {
		case evt := <-events:
			if evt.Type != typ {
				t.Fatalf("expected event %q, got %q", typ, evt.Type)
			}
			if evt.Type == "log" {
				var payload logstream.LogRecord
				if err := json.Unmarshal([]byte(evt.Data), &payload); err != nil {
					t.Fatalf("decode log payload: %v", err)
				}
				if payload.Line != "node-start" {
					t.Fatalf("unexpected log line: %s", payload.Line)
				}
			}
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for %s", typ)
		}
	}
}

type sseEvent struct {
	ID   string
	Type string
	Data string
}

func readEvent(r *bufio.Reader) (sseEvent, error) {
	var evt sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return evt, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if evt.Type == "" && evt.Data == "" && evt.ID == "" {
				continue
			}
			return evt, nil
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			evt.Type = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if evt.Data != "" {
				evt.Data += "\n"
			}
			evt.Data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case strings.HasPrefix(line, "id:"):
			evt.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		}
	}
}
