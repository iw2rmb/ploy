package jobs

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestFollowCommandStreamsJobLogs(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	server := newJobSSEServer(t, jobID, []sseEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"stdout","line":"building"}`},
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:01Z","stream":"stderr","line":"warning: deprecated"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	var buf bytes.Buffer
	cmd := FollowCommand{
		Client:  stream.Client{HTTPClient: server.Client()},
		BaseURL: baseURL,
		JobID:   jobID,
		Format:  logs.FormatStructured,
		Output:  &buf,
	}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("FollowCommand.Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-03-01T10:00:00Z stdout building") {
		t.Fatalf("expected structured stdout line, got: %q", out)
	}
	if !strings.Contains(out, "2026-03-01T10:00:01Z stderr warning: deprecated") {
		t.Fatalf("expected structured stderr line, got: %q", out)
	}
}

func TestFollowCommandRawFormat(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	server := newJobSSEServer(t, jobID, []sseEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"stdout","line":"raw output"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	var buf bytes.Buffer
	cmd := FollowCommand{
		Client:  stream.Client{HTTPClient: server.Client()},
		BaseURL: baseURL,
		JobID:   jobID,
		Format:  logs.FormatRaw,
		Output:  &buf,
	}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("FollowCommand.Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "raw output") {
		t.Fatalf("expected raw line, got: %q", out)
	}
	if strings.Contains(out, "2026-03-01T10:00:00Z") {
		t.Fatalf("raw format should not include timestamp, got: %q", out)
	}
}

func TestFollowCommandRetention(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	server := newJobSSEServer(t, jobID, []sseEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"stdout","line":"hello"}`},
		{event: "retention", data: `{"retained":true,"ttl":"72h","expires_at":"2026-03-04T10:00:00Z","bundle_cid":"bafy-job-bundle"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	var buf bytes.Buffer
	cmd := FollowCommand{
		Client:  stream.Client{HTTPClient: server.Client()},
		BaseURL: baseURL,
		JobID:   jobID,
		Format:  logs.FormatStructured,
		Output:  &buf,
	}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("FollowCommand.Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Retention: retained ttl=72h expires=2026-03-04T10:00:00Z cid=bafy-job-bundle") {
		t.Fatalf("expected retention summary, got: %q", out)
	}
}

func TestFollowCommandValidation(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("http://localhost:9999")

	tests := []struct {
		name    string
		cmd     FollowCommand
		wantErr string
	}{
		{
			name: "missing job id",
			cmd: FollowCommand{
				Client:  stream.Client{HTTPClient: http.DefaultClient},
				BaseURL: baseURL,
			},
			wantErr: "job id required",
		},
		{
			name: "missing base url",
			cmd: FollowCommand{
				Client: stream.Client{HTTPClient: http.DefaultClient},
				JobID:  domaintypes.NewJobID(),
			},
			wantErr: "base url required",
		},
		{
			name: "invalid format",
			cmd: FollowCommand{
				Client:  stream.Client{HTTPClient: http.DefaultClient},
				BaseURL: baseURL,
				JobID:   domaintypes.NewJobID(),
				Format:  "yaml",
			},
			wantErr: "invalid format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestFollowCommandUsesJobEndpoint(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	expectedPath := "/v1/jobs/" + jobID.String() + "/logs"

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "event: done\ndata: {\"status\":\"completed\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)

	var buf bytes.Buffer
	cmd := FollowCommand{
		Client:  stream.Client{HTTPClient: server.Client()},
		BaseURL: baseURL,
		JobID:   jobID,
		Output:  &buf,
	}

	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("FollowCommand.Run: %v", err)
	}

	if requestedPath != expectedPath {
		t.Fatalf("expected request to %q, got %q", expectedPath, requestedPath)
	}
}

type sseEvent struct {
	event string
	data  string
}

func newJobSSEServer(t *testing.T, jobID domaintypes.JobID, events []sseEvent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/v1/jobs/" + jobID.String() + "/logs"
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		for _, evt := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.event, evt.data)
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
}
