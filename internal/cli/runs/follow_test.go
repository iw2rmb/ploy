package runs

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
)

func TestFollowCommandReconnects(t *testing.T) {
	t.Helper()
	plan := []followPlan{
		{
			events:     []testEvent{{event: "log", data: `{"timestamp":"2025-10-22T11:00:00Z","stream":"stdout","line":"first"}`}},
			closeAfter: true,
		},
		{
			events: []testEvent{
				{event: "log", data: `{"timestamp":"2025-10-22T11:00:01Z","stream":"stdout","line":"second"}`},
				{event: "retention", data: `{"retained":true,"ttl":"48h","expires_at":"2025-10-24T11:00:01Z","bundle_cid":"bafy-job"}`},
				{event: "done", data: `{"status":"completed"}`},
			},
		},
	}
	server := newFollowStreamServer(t, plan)
	defer server.Close()

	httpClient := server.Client()
	httpClient.Timeout = 0

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	buf := &bytes.Buffer{}
	cmd := FollowCommand{
		Client: stream.Client{
			HTTPClient: httpClient,
			MaxRetries: 3,
			// Backoff is handled by the shared SSE backoff policy.
		},
		BaseURL: baseURL,
		JobID:   "job-1",
		Format:  logs.FormatStructured, // Use canonical logs.Format directly.
		Output:  buf,
	}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("run follow command: %v", err)
	}
	got := buf.String()
	want := "2025-10-22T11:00:00Z stdout first\n2025-10-22T11:00:01Z stdout second\nRetention: retained ttl=48h expires=2025-10-24T11:00:01Z cid=bafy-job\n"
	if got != want {
		t.Fatalf("unexpected output\nwant: %q\ngot:  %q", want, got)
	}
}

type followPlan struct {
	events     []testEvent
	closeAfter bool
}

type testEvent struct {
	event string
	data  string
}

func newFollowStreamServer(t *testing.T, plans []followPlan) *httptest.Server {
	t.Helper()
	var idx int
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/job-1/logs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		plan := plans[idx]
		if idx < len(plans)-1 {
			idx++
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer missing flusher")
		}
		for _, evt := range plan.events {
			if evt.event != "" {
				_, _ = w.Write([]byte("event: " + evt.event + "\n"))
			}
			if evt.data != "" {
				_, _ = w.Write([]byte("data: " + evt.data + "\n"))
			}
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
		if plan.closeAfter {
			return
		}
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}
