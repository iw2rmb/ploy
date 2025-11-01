package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

func TestNodeJobLogsSnapshotAndEntries(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
`)
	hub := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 32})

	server, err := httpserver.New(httpserver.Options{Config: cfg, Streams: hub, Status: &stubStatus{}})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	app := server.App()

	ctx := context.Background()
	_ = hub.PublishLog(ctx, "job-logs", logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Stream: "stdout", Line: "first"})
	_ = hub.PublishStatus(ctx, "job-logs", logstream.Status{Status: "completed"})

	req := httptest.NewRequest(http.MethodGet, "/v1/node/jobs/job-logs/logs/snapshot", nil)
	resp, err := app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test(): %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, _ := body["events"].([]any)
	if len(events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(events))
	}

	// Append an entry and observe it via snapshot
	post := httptest.NewRequest(http.MethodPost, "/v1/node/jobs/job-extra/logs/entries", strings.NewReader(`{"stream":"stderr","line":"oops"}`))
	post.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(post, 1000)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("entries status=%d want 202", resp.StatusCode)
	}

	snap := httptest.NewRequest(http.MethodGet, "/v1/node/jobs/job-extra/logs/snapshot", nil)
	resp, err = app.Test(snap, 1000)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status=%d", resp.StatusCode)
	}
	body = map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, _ = body["events"].([]any)
	if len(events) == 0 {
		t.Fatalf("expected events after entries post")
	}
	// Verify presence of the just-posted log frame with expected payload
	found := false
	for _, e := range events {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] != "log" {
			continue
		}
		data, ok := m["data"].(map[string]any)
		if !ok {
			continue
		}
		if data["line"] == "oops" && data["stream"] == "stderr" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("snapshot missing posted log entry")
	}
}
