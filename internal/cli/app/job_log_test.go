package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestJobStatusJSONOutput(t *testing.T) {
	jobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/jobs/"+jobID.String()+"/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":        jobID.String(),
			"run_id":        runID.String(),
			"repo_id":       repoID.String(),
			"attempt":       1,
			"name":          "mig-0",
			"job_type":      "mig",
			"status":        "Running",
			"job_image":     "ghcr.io/acme/mig:1",
			"node_id":       nil,
			"exit_code":     nil,
			"started_at":    nil,
			"finished_at":   nil,
			"duration_ms":   0,
			"repo_sha_in":   "0123456789abcdef0123456789abcdef01234567",
			"repo_sha_out":  "",
			"repo_sha_in8":  "01234567",
			"repo_sha_out8": "",
		})
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "status", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job status: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q (err=%v)", buf.String(), err)
	}
	if got := parsed["job_id"]; got != jobID.String() {
		t.Fatalf("job_id = %#v, want %s", got, jobID.String())
	}
	if got := parsed["status"]; got != "Running" {
		t.Fatalf("status = %#v, want Running", got)
	}
	if got := parsed["run_id"]; got != runID.String() {
		t.Fatalf("run_id = %#v, want %s", got, runID.String())
	}
}

func TestJobLogStructuredOutput(t *testing.T) {
	jobID := domaintypes.NewJobID()
	server := newJobStreamingServer(t, jobID, []sseTestEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:00Z","stream":"stdout","line":"Step started"}`},
		{event: "log", data: `{"timestamp":"2026-03-01T10:00:01Z","stream":"stderr","line":"warning: slow retry"}`},
		{event: "retention", data: `{"retained":true,"ttl":"72h","expires_at":"2026-03-04T10:00:00Z","bundle_cid":"bafy-ret-bundle"}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "log", "--format", "structured", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job log: %v", err)
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

func TestJobLogRawOutput(t *testing.T) {
	jobID := domaintypes.NewJobID()
	server := newJobStreamingServer(t, jobID, []sseTestEvent{
		{event: "log", data: `{"timestamp":"2026-03-01T10:05:00Z","stream":"stdout","line":"ready"}`},
		{event: "log", data: `{"timestamp":"2026-03-01T10:05:01Z","stream":"stderr","line":"warn"}`},
		{event: "retention", data: `{"retained":false,"ttl":"","expires_at":""}`},
		{event: "done", data: `{"status":"completed"}`},
	})
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "log", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job log raw: %v", err)
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

func TestJobLogInvalidFormat(t *testing.T) {
	clienv.UseControlPlaneEnv(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "log", "--format", "yaml", "job-123"}, buf)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestJobLogFollowReconnects(t *testing.T) {
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

	clienv.UseControlPlaneEnv(t, server.URL)

	buf := &bytes.Buffer{}
	err := executeCmd([]string{"job", "log", "--follow", jobID.String()}, buf)
	if err != nil {
		t.Fatalf("job log --follow reconnect: %v", err)
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
	if cfg.jobID != "" {
		streamPath = "/v1/jobs/" + cfg.jobID + "/logs"
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
