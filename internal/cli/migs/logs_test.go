package migs

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

func TestLogsCommandStreamsStructured(t *testing.T) {
	t.Helper()
	server := newMigsStreamServer(t, []testEvent{
		{event: "log", data: `{"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"line-1"}`},
		{event: "retention", data: `{"retained":true,"ttl":"72h","expires_at":"2025-10-25T10:00:00Z","bundle_cid":"bafy-test"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	httpClient := server.Client()
	httpClient.Timeout = 0

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	buf := &bytes.Buffer{}
	cmd := LogsCommand{
		Client: stream.Client{
			HTTPClient: httpClient,
			MaxRetries: 1,
			// Backoff is handled by the shared SSE backoff policy.
		},
		BaseURL: baseURL,
		RunID:   "test",
		Format:  logs.FormatStructured, // Use canonical logs.Format directly.
		Output:  buf,
	}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("run logs command: %v", err)
	}
	got := buf.String()
	want := "2025-10-22T10:00:00Z stdout line-1\nRetention: retained ttl=72h expires=2025-10-25T10:00:00Z cid=bafy-test\n"
	if got != want {
		t.Fatalf("unexpected output\nwant: %q\ngot:  %q", want, got)
	}
}

type testEvent struct {
	event string
	data  string
}

func newMigsStreamServer(t *testing.T, events []testEvent) *httptest.Server {
	t.Helper()
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/test/logs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer missing flusher")
		}
		for _, evt := range events {
			if evt.event != "" {
				_, _ = w.Write([]byte("event: " + evt.event + "\n"))
			}
			if evt.data != "" {
				_, _ = w.Write([]byte("data: " + evt.data + "\n"))
			}
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}
