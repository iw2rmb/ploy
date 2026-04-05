package migs

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestLogsCommandStreamsLifecycle(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	server := newMigsStreamServer(t, []testEvent{
		{event: "run", data: `{"run_id":"` + runID.String() + `","state":"running"}`},
		{event: "stage", data: `{"run_id":"` + runID.String() + `","stage":{"state":"running","current_job_id":"` + domaintypes.NewJobID().String() + `"}}`},
		{event: "run", data: `{"run_id":"` + runID.String() + `","state":"succeeded"}`},
	})
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	buf := &bytes.Buffer{}
	cmd := LogsCommand{
		Client: stream.Client{
			HTTPClient: server.Client(),
			MaxRetries: 1,
		},
		BaseURL: baseURL,
		RunID:   runID,
		Output:  buf,
	}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("run logs command: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "succeeded") {
		t.Fatalf("expected lifecycle output with succeeded state, got: %q", got)
	}
}

func TestLogsCommandDoneSentinel(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	server := newMigsStreamServer(t, []testEvent{
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)
	buf := &bytes.Buffer{}
	cmd := LogsCommand{
		Client: stream.Client{
			HTTPClient: server.Client(),
			MaxRetries: 0,
		},
		BaseURL: baseURL,
		RunID:   runID,
		Output:  buf,
	}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("run logs command: %v", err)
	}
}

type testEvent struct {
	event string
	data  string
}

func newMigsStreamServer(t *testing.T, events []testEvent) *httptest.Server {
	t.Helper()
	handler := func(w http.ResponseWriter, r *http.Request) {
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
