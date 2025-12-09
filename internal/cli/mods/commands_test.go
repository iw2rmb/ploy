package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// newTestLogPrinter creates a LogPrinter for testing that writes to the provided writer.
// Uses structured format to enable verification of enriched fields.
func newTestLogPrinter(w io.Writer) *logs.Printer {
	return logs.NewPrinter(logs.FormatStructured, w)
}

func TestInspectAndArtifactsCommands(t *testing.T) {
	run := modsapi.RunSummary{
		RunID: domaintypes.RunID("t1"),
		State: modsapi.RunStateSucceeded,
		Stages: map[string]modsapi.StageStatus{
			"build": {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"bin": "cid1"}},
			"test":  {State: modsapi.StageStateSucceeded},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return RunSummary directly — the canonical response shape.
		_ = json.NewEncoder(w).Encode(run)
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	// Inspect prints one-line summary.
	var out bytes.Buffer
	if err := (InspectCommand{Client: srv.Client(), BaseURL: base, RunID: "t1", Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("inspect run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected inspect to write output")
	}

	// Artifacts prints per-stage artifacts.
	out.Reset()
	if err := (ArtifactsCommand{Client: srv.Client(), BaseURL: base, RunID: "t1", Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("artifacts run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected artifacts to write output")
	}
}

func TestInspectCommand_GateSummary(t *testing.T) {
	// Test that inspect command displays gate summary when present in metadata.
	tests := []struct {
		name         string
		run          modsapi.RunSummary
		wantContains string
	}{
		{
			name: "gate summary present",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("t-gate-1"),
				State: modsapi.RunStateSucceeded,
				Metadata: map[string]string{
					"gate_summary": "passed duration=1234ms",
				},
			},
			wantContains: "Gate: passed duration=1234ms",
		},
		{
			name: "gate summary and MR URL",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("t-gate-2"),
				State: modsapi.RunStateSucceeded,
				Metadata: map[string]string{
					"mr_url":       "https://gitlab.com/org/repo/-/merge_requests/42",
					"gate_summary": "failed pre-gate duration=567ms",
				},
			},
			wantContains: "Gate: failed pre-gate duration=567ms",
		},
		{
			// Verifies CLI displays post-mod (final_gate) failure correctly.
			// The gate_summary is populated by the server from stats.GateSummary(),
			// which prioritizes final_gate over pre_gate. This test ensures the CLI
			// renders the "failed final-gate ..." format without alteration.
			name: "final gate failed after mods",
			run: modsapi.RunSummary{
				RunID: domaintypes.RunID("t-gate-final-failed"),
				State: modsapi.RunStateFailed,
				Metadata: map[string]string{
					"gate_summary": "failed final-gate duration=2345ms",
				},
			},
			wantContains: "Gate: failed final-gate duration=2345ms",
		},
		{
			name: "no gate summary",
			run: modsapi.RunSummary{
				RunID:    domaintypes.RunID("t-no-gate"),
				State:    modsapi.RunStateSucceeded,
				Metadata: map[string]string{},
			},
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return RunSummary directly — the canonical response shape.
				_ = json.NewEncoder(w).Encode(tt.run)
			}))
			defer srv.Close()
			base, _ := url.Parse(srv.URL)

			var out bytes.Buffer
			cmd := InspectCommand{
				Client:  srv.Client(),
				BaseURL: base,
				RunID:   tt.run.RunID,
				Output:  &out,
			}
			if err := cmd.Run(context.Background()); err != nil {
				t.Fatalf("inspect run: %v", err)
			}

			output := out.String()
			if tt.wantContains != "" {
				if !bytes.Contains([]byte(output), []byte(tt.wantContains)) {
					t.Errorf("output missing expected gate summary\ngot: %q\nwant substring: %q", output, tt.wantContains)
				}
			} else {
				// When no gate summary, ensure "Gate:" line is not present.
				if bytes.Contains([]byte(output), []byte("Gate:")) {
					t.Errorf("output unexpectedly contains gate summary\ngot: %q", output)
				}
			}
		})
	}
}

func TestCancelResumeSubmitCommands(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Server returns 201 Created with canonical submit response.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		}{RunID: "t2", Status: "pending"})
	})
	mux.HandleFunc("/v1/mods/t2/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/v1/mods/t2/resume", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	// Submit
	sum, err := (SubmitCommand{Client: srv.Client(), BaseURL: base, Request: modsapi.RunSubmitRequest{RunID: domaintypes.RunID("t2")}}).Run(context.Background())
	if err != nil || string(sum.RunID) != "t2" {
		t.Fatalf("submit err=%v run=%+v", err, sum)
	}
	// Cancel
	if err := (CancelCommand{Client: srv.Client(), BaseURL: base, RunID: "t2"}).Run(context.Background()); err != nil {
		t.Fatalf("cancel err=%v", err)
	}
	// Resume
	if err := (ResumeCommand{Client: srv.Client(), BaseURL: base, RunID: "t2"}).Run(context.Background()); err != nil {
		t.Fatalf("resume err=%v", err)
	}
}

func TestEventsCommandStreamsToTerminal(t *testing.T) {
	tests := []struct {
		name          string
		terminalState modsapi.RunState
	}{
		{"succeeded", modsapi.RunStateSucceeded},
		{"cancelled", modsapi.RunStateCancelled},
		{"failed", modsapi.RunStateFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SSE server emits a terminal run event (RunSummary directly, no wrapper).
			sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				runSummary := modsapi.RunSummary{RunID: domaintypes.RunID("t3"), State: tt.terminalState}
				b, _ := json.Marshal(runSummary)
				_, _ = w.Write([]byte("event: run\n"))
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(b)
				_, _ = w.Write([]byte("\n\n"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(10 * time.Millisecond)
			}))
			defer sse.Close()
			base, _ := url.Parse(sse.URL)

			cli := stream.Client{HTTPClient: sse.Client(), MaxRetries: 0}
			state, err := (EventsCommand{Client: cli, BaseURL: base, RunID: "t3"}).Run(context.Background())
			if err != nil {
				t.Fatalf("events run err=%v", err)
			}
			if state != tt.terminalState {
				t.Fatalf("events final state=%s, want %s", state, tt.terminalState)
			}
		})
	}
}

func TestModsCommandsErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad req"}`))
	})
	mux.HandleFunc("/v1/mods/t/cancel", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	mux.HandleFunc("/v1/mods/t/resume", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	mux.HandleFunc("/v1/mods/t", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	// Submit
	if _, err := (SubmitCommand{Client: srv.Client(), BaseURL: base}).Run(context.Background()); err == nil {
		t.Fatal("expected submit error")
	}
	// Cancel
	if err := (CancelCommand{Client: srv.Client(), BaseURL: base, RunID: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected cancel error")
	}
	// Resume
	if err := (ResumeCommand{Client: srv.Client(), BaseURL: base, RunID: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected resume error")
	}
	// Inspect
	if err := (InspectCommand{Client: srv.Client(), BaseURL: base, RunID: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected inspect error")
	}
}

func TestSimplePrinterFormats(t *testing.T) {
	var b bytes.Buffer
	p := SimplePrinter{out: &b}
	p.Run(modsapi.RunSummary{RunID: domaintypes.RunID("t1"), State: modsapi.RunStateRunning})
	p.Stage(modsapi.StageStatus{State: modsapi.StageStateFailed, Attempts: 2, CurrentJobID: domaintypes.JobID("j1"), LastError: "boom"})
	if b.Len() == 0 {
		t.Fatalf("expected printer output")
	}
}

// TestEventsCommandWithLogPrinter verifies that EventsCommand renders log events
// using the shared LogPrinter when configured (unified log streaming for mod run --follow).
func TestEventsCommandWithLogPrinter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		events     []string // SSE event lines to send
		wantLog    string   // expected substring in log output
		wantFinal  modsapi.RunState
		wantNodeID bool // whether node= context should appear
	}{
		{
			name: "log event with enriched fields",
			events: []string{
				"event: run\ndata: {\"run_id\":\"t-log\",\"state\":\"running\"}\n\n",
				"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:00Z\",\"stream\":\"stdout\",\"line\":\"Build started\",\"node_id\":\"node-1\",\"job_id\":\"job-1\",\"mod_type\":\"mod\",\"step_index\":100}\n\n",
				"event: run\ndata: {\"run_id\":\"t-log\",\"state\":\"succeeded\"}\n\n",
			},
			wantLog:    "Build started",
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: true,
		},
		{
			name: "log event without enriched fields",
			events: []string{
				"event: run\ndata: {\"run_id\":\"t-log2\",\"state\":\"running\"}\n\n",
				"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:01Z\",\"stream\":\"stderr\",\"line\":\"Warning\"}\n\n",
				"event: run\ndata: {\"run_id\":\"t-log2\",\"state\":\"succeeded\"}\n\n",
			},
			wantLog:    "Warning",
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: false,
		},
		{
			name: "retention event recorded",
			events: []string{
				"event: run\ndata: {\"run_id\":\"t-ret\",\"state\":\"running\"}\n\n",
				"event: retention\ndata: {\"retained\":true,\"ttl\":\"24h\",\"expires_at\":\"2025-10-23T10:00:00Z\",\"bundle_cid\":\"bafy-bundle\"}\n\n",
				"event: run\ndata: {\"run_id\":\"t-ret\",\"state\":\"succeeded\"}\n\n",
			},
			wantLog:    "retained", // retention summary is printed
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fl, ok := w.(http.Flusher)
				if !ok {
					t.Fatal("no flusher")
				}
				fl.Flush()

				for _, evt := range tt.events {
					_, _ = w.Write([]byte(evt))
					fl.Flush()
					time.Sleep(2 * time.Millisecond)
				}
			}))
			defer sse.Close()

			base, _ := url.Parse(sse.URL)
			var buf bytes.Buffer

			// Create a LogPrinter to capture log output.
			logPrinter := newTestLogPrinter(&buf)

			cmd := EventsCommand{
				Client:     stream.Client{HTTPClient: sse.Client(), MaxRetries: 0},
				BaseURL:    base,
				RunID:      "t-test",
				Output:     &buf,
				LogPrinter: logPrinter,
			}

			state, err := cmd.Run(context.Background())
			if err != nil {
				t.Fatalf("events run: %v", err)
			}
			if state != tt.wantFinal {
				t.Errorf("state=%s, want %s", state, tt.wantFinal)
			}

			out := buf.String()
			if tt.wantLog != "" && !bytes.Contains([]byte(out), []byte(tt.wantLog)) {
				t.Errorf("output missing %q, got: %s", tt.wantLog, out)
			}
			if tt.wantNodeID && !bytes.Contains([]byte(out), []byte("node=")) {
				t.Errorf("output missing node= context, got: %s", out)
			}
		})
	}
}

// TestEventsCommandWithoutLogPrinter verifies backward compatibility: when
// LogPrinter is nil, log events are ignored and only run/stage events are processed.
func TestEventsCommandWithoutLogPrinter(t *testing.T) {
	t.Parallel()

	sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		fl.Flush()

		// Send run, log, and run events.
		events := []string{
			"event: run\ndata: {\"run_id\":\"t-nolog\",\"state\":\"running\"}\n\n",
			"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:00Z\",\"stream\":\"stdout\",\"line\":\"Should be ignored\"}\n\n",
			"event: run\ndata: {\"run_id\":\"t-nolog\",\"state\":\"succeeded\"}\n\n",
		}
		for _, evt := range events {
			_, _ = w.Write([]byte(evt))
			fl.Flush()
			time.Sleep(2 * time.Millisecond)
		}
	}))
	defer sse.Close()

	base, _ := url.Parse(sse.URL)
	var buf bytes.Buffer

	cmd := EventsCommand{
		Client:     stream.Client{HTTPClient: sse.Client(), MaxRetries: 0},
		BaseURL:    base,
		RunID:      "t-nolog",
		Output:     &buf,
		LogPrinter: nil, // No LogPrinter configured.
	}

	state, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("events run: %v", err)
	}
	if state != modsapi.RunStateSucceeded {
		t.Errorf("state=%s, want succeeded", state)
	}

	out := buf.String()
	// Log message should NOT appear since LogPrinter is nil.
	if bytes.Contains([]byte(out), []byte("Should be ignored")) {
		t.Errorf("log message should not appear when LogPrinter is nil, got: %s", out)
	}
	// Ticket state should still appear via SimplePrinter.
	if !bytes.Contains([]byte(out), []byte("t-nolog")) {
		t.Errorf("run ID should appear in output, got: %s", out)
	}
}
