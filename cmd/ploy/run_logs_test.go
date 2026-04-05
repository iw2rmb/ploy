package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunLogsLifecycleOutput(t *testing.T) {
	t.Helper()
	runID := domaintypes.NewRunID()
	server := newStreamingServer(t, streamingServerConfig{
		migRunID: runID,
		logEvents: []sseTestEvent{
			{event: "run", data: `{"state":"running"}`},
			{event: "stage", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"info","line":"step started"}`},
			{event: "done", data: `{"status":"completed"}`},
		},
	})
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"run", "logs", runID.String()}, buf)
	if err != nil {
		t.Fatalf("run logs: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[run] state=running") {
		t.Fatalf("expected run lifecycle event, got: %q", out)
	}
	if !strings.Contains(out, "[stage]") || !strings.Contains(out, "step started") {
		t.Fatalf("expected stage lifecycle event, got: %q", out)
	}
}

func TestRunLogsRequiresRunID(t *testing.T) {
	t.Helper()
	clienv.UseServerDescriptor(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"run", "logs"}, buf)
	if err == nil {
		t.Fatal("expected error when run id is missing")
	}
	if !strings.Contains(err.Error(), "run id") {
		t.Fatalf("expected run id error, got %v", err)
	}
}

func TestJobFollowStructuredOutput(t *testing.T) {
	t.Helper()
	jobID := domaintypes.NewJobID()
	server := newJobStreamingServer(t, jobID, []sseTestEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"stdout","line":"Step started"}`},
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:01Z","stream":"stderr","line":"warning: slow retry"}`},
		{event: "retention", data: `{"retained":true,"ttl":"72h","expires_at":"2026-03-04T10:00:00Z","bundle_cid":"bafy-ret-bundle"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "follow", "--format", "structured", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job follow: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-03-01T10:00:00Z stdout Step started") {
		t.Fatalf("expected structured stdout line, got: %q", out)
	}
	if !strings.Contains(out, "2026-03-01T10:00:01Z stderr warning: slow retry") {
		t.Fatalf("expected structured stderr line, got: %q", out)
	}
	if !strings.Contains(out, "Retention: retained ttl=72h expires=2026-03-04T10:00:00Z cid=bafy-ret-bundle") {
		t.Fatalf("expected retention summary, got: %q", out)
	}
}

func TestJobFollowRawOutput(t *testing.T) {
	t.Helper()
	jobID := domaintypes.NewJobID()
	server := newJobStreamingServer(t, jobID, []sseTestEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:05:00Z","stream":"stdout","line":"ready"}`},
		{event: "log", data: `{"timestamp":"2026-03-01T10:05:01Z","stream":"stderr","line":"warn"}`},
		{event: "retention", data: `{"retained":false,"ttl":"","expires_at":""}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "follow", "--format", "raw", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job follow raw: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ready\n") {
		t.Fatalf("expected raw line 'ready', got: %q", out)
	}
	if !strings.Contains(out, "warn\n") {
		t.Fatalf("expected raw line 'warn', got: %q", out)
	}
	if strings.Contains(out, "2026-03-01T10:05:00Z") {
		t.Fatalf("raw format should not include timestamp, got: %q", out)
	}
}

func TestJobFollowRequiresJobID(t *testing.T) {
	t.Helper()
	clienv.UseServerDescriptor(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "follow"}, buf)
	if err == nil {
		t.Fatal("expected error when job id is missing")
	}
	if !strings.Contains(err.Error(), "job id") {
		t.Fatalf("expected job id error, got %v", err)
	}
}

func TestJobFollowInvalidFormat(t *testing.T) {
	t.Helper()
	clienv.UseServerDescriptor(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "follow", "--format", "yaml", "job-123"}, buf)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestJobFollowReconnects(t *testing.T) {
	t.Helper()
	jobID := domaintypes.NewJobID()
	server := newJobStreamingServer(t, jobID, nil)
	// Override with reconnect plan
	server.Close()

	server = newStreamingServer(t, streamingServerConfig{
		jobID: jobID.String(),
		reconnects: []streamReconnectPlan{
			{
				events: []sseTestEvent{
					{event: "log", data: `{"timestamp":"2026-03-01T11:00:00Z","stream":"stdout","line":"first"}`},
				},
				closeAfter: true,
			},
			{
				events: []sseTestEvent{
					{event: "log", data: `{"timestamp":"2026-03-01T11:00:01Z","stream":"stdout","line":"second"}`},
					{event: "done", data: `{"status":"completed"}`},
				},
			},
		},
	})
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "follow", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job follow reconnect: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "first") {
		t.Fatalf("expected 'first' in output, got: %q", out)
	}
	if !strings.Contains(out, "second") {
		t.Fatalf("expected 'second' in output, got: %q", out)
	}
}

type streamingServerConfig struct {
	migRunID   domaintypes.RunID
	jobID      string
	logEvents  []sseTestEvent
	reconnects []streamReconnectPlan
}

type streamReconnectPlan struct {
	events     []sseTestEvent
	closeAfter bool
}

type sseTestEvent struct {
	event string
	data  string
}

func newStreamingServer(t *testing.T, cfg streamingServerConfig) *httptest.Server {
	t.Helper()
	var (
		mu          sync.Mutex
		connectionN int
	)
	streamPath := ""
	if !cfg.migRunID.IsZero() {
		streamPath = fmt.Sprintf("/v1/runs/%s/logs", cfg.migRunID.String())
	}
	if cfg.jobID != "" {
		streamPath = fmt.Sprintf("/v1/jobs/%s/logs", cfg.jobID)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case streamPath:
			mu.Lock()
			connectionN++
			current := connectionN
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("response writer does not support flushing")
			}
			events := cfg.logEvents
			shouldClose := true
			if len(cfg.reconnects) > 0 {
				index := current - 1
				if index >= len(cfg.reconnects) {
					index = len(cfg.reconnects) - 1
				}
				plan := cfg.reconnects[index]
				events = plan.events
				shouldClose = plan.closeAfter
			}
			for _, evt := range events {
				_, _ = fmt.Fprintf(w, "event: %s\n", evt.event)
				scanner := bufio.NewScanner(strings.NewReader(evt.data))
				for scanner.Scan() {
					_, _ = fmt.Fprintf(w, "data: %s\n", scanner.Text())
				}
				_, _ = fmt.Fprint(w, "\n")
				flusher.Flush()
			}
			if shouldClose {
				return
			}
			<-r.Context().Done()
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(func() {
		server.Close()
	})
	return server
}

func newJobStreamingServer(t *testing.T, jobID domaintypes.JobID, events []sseTestEvent) *httptest.Server {
	t.Helper()
	return newStreamingServer(t, streamingServerConfig{
		jobID:     jobID.String(),
		logEvents: events,
	})
}
