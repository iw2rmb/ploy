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
)

func TestModsLogsStructuredOutput(t *testing.T) {
	t.Helper()
	server := newStreamingServer(t, streamingServerConfig{
		modTicket: "ticket-123",
		logEvents: []sseTestEvent{
			{
				event: "log",
				data:  `{"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"Step started"}`,
			},
			{
				event: "log",
				data:  `{"timestamp":"2025-10-22T10:00:01Z","stream":"stderr","line":"warning: slow retry"}`,
			},
			{
				event: "retention",
				data:  `{"retained":true,"ttl":"72h","expires_at":"2025-10-25T10:00:00Z","bundle_cid":"bafy-ret-bundle"}`,
			},
			{
				event: "done",
				data:  `{"status":"completed"}`,
			},
		},
	})
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := execute([]string{"mods", "logs", "--format", "structured", "ticket-123"}, buf)
	if err != nil {
		t.Fatalf("mods logs: %v", err)
	}
	expect := loadGolden(t, "mods_logs_structured.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("mods logs structured mismatch:\n%s", diff)
	}
}

func TestModsLogsRawOutput(t *testing.T) {
	t.Helper()
	server := newStreamingServer(t, streamingServerConfig{
		modTicket: "ticket-raw",
		logEvents: []sseTestEvent{
			{event: "log", data: `{"timestamp":"2025-10-22T10:05:00Z","stream":"stdout","line":"ready"}`},
			{event: "log", data: `{"timestamp":"2025-10-22T10:05:01Z","stream":"stderr","line":"warn"}`},
			{event: "retention", data: `{"retained":false,"ttl":"","expires_at":"","bundle_cid":""}`},
			{event: "done", data: `{"status":"completed"}`},
		},
	})
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := execute([]string{"mods", "logs", "--format", "raw", "ticket-raw"}, buf)
	if err != nil {
		t.Fatalf("mods logs raw: %v", err)
	}
	expect := loadGolden(t, "mods_logs_raw.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("mods logs raw mismatch:\n%s", diff)
	}
}

func TestModsLogsRequiresTicket(t *testing.T) {
	t.Helper()
	useServerDescriptor(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := execute([]string{"mods", "logs"}, buf)
	if err == nil {
		t.Fatal("expected error when ticket is missing")
	}
	if !strings.Contains(err.Error(), "ticket") {
		t.Fatalf("expected ticket error, got %v", err)
	}
}

func TestModsLogsInvalidFormat(t *testing.T) {
	t.Helper()
	useServerDescriptor(t, "http://example.invalid")

	buf := &bytes.Buffer{}
	err := execute([]string{"mods", "logs", "--format", "yaml", "ticket-123"}, buf)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestJobsFollowReconnects(t *testing.T) {
	t.Helper()
	server := newStreamingServer(t, streamingServerConfig{
		jobID: "job-42",
		reconnects: []streamReconnectPlan{
			{
				events: []sseTestEvent{
					{event: "log", data: `{"timestamp":"2025-10-22T11:00:00Z","stream":"stdout","line":"first"}`},
				},
				closeAfter: true,
			},
			{
				events: []sseTestEvent{
					{event: "log", data: `{"timestamp":"2025-10-22T11:00:01Z","stream":"stdout","line":"second"}`},
					{event: "done", data: `{"status":"completed"}`},
				},
			},
		},
	})
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	err := execute([]string{"runs", "follow", "job-42"}, buf)
	if err != nil {
		t.Fatalf("runs follow: %v", err)
	}
	expect := loadGolden(t, "jobs_follow_structured.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("runs follow output mismatch:\n%s", diff)
	}
}

type streamingServerConfig struct {
	modTicket  string
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
	if cfg.modTicket != "" {
		streamPath = fmt.Sprintf("/v1/mods/%s/events", cfg.modTicket)
	}
	if cfg.jobID != "" {
		streamPath = fmt.Sprintf("/v1/mods/%s/events", cfg.jobID)
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

// env helper removed; tests now use useServerDescriptor to point CLI to the test server.
