package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// newTestLogPrinter creates a LogPrinter for testing that writes to the provided writer.
// Uses structured format to enable verification of enriched fields.
func newTestLogPrinter(w io.Writer) *logs.Printer {
	return logs.NewPrinter(logs.FormatStructured, w)
}

func TestArtifactsCommand(t *testing.T) {
	runID := domaintypes.NewRunID()
	buildJobID := domaintypes.NewJobID()
	testJobID := domaintypes.NewJobID()

	run := modsapi.RunSummary{
		RunID: runID,
		State: modsapi.RunStateSucceeded,
		Stages: map[domaintypes.JobID]modsapi.StageStatus{
			buildJobID: {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"bin": "cid1"}},
			testJobID:  {State: modsapi.StageStateSucceeded},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return RunSummary directly — the canonical response shape.
		_ = json.NewEncoder(w).Encode(run)
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	var out bytes.Buffer
	if err := (ArtifactsCommand{Client: srv.Client(), BaseURL: base, RunID: runID, Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("artifacts run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected artifacts to write output")
	}
}

func TestCancelResumeSubmitCommands(t *testing.T) {
	runID := domaintypes.NewRunID()
	modID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	runIDStr := runID.String()
	modIDStr := modID.String()
	specIDStr := specID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Server returns 201 Created with {run_id, mod_id, spec_id}.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			RunID  string `json:"run_id"`
			MigID  string `json:"mig_id"`
			SpecID string `json:"spec_id"`
		}{
			RunID:  runIDStr,
			MigID:  modIDStr,
			SpecID: specIDStr,
		})
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/status", func(w http.ResponseWriter, r *http.Request) {
		// Canonical RunSummary response shape for status.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
			RunID:      runID,
			State:      modsapi.RunStatePending,
			Repository: "https://example.com/repo.git",
			Metadata: map[string]string{
				"repo_base_ref":   "main",
				"repo_target_ref": "feature",
			},
			Stages: make(map[domaintypes.JobID]modsapi.StageStatus),
		})
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	// Submit
	sum, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: modsapi.RunSubmitRequest{
			RepoURL:   domaintypes.RepoURL("https://example.com/repo.git"),
			BaseRef:   domaintypes.GitRef("main"),
			TargetRef: domaintypes.GitRef("feature"),
			Spec:      []byte("{}"),
		},
	}).Run(context.Background())
	if err != nil || sum.RunID != runID {
		t.Fatalf("submit err=%v run=%+v", err, sum)
	}
	// Cancel
	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err != nil {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestSubmitCommand_InvalidRepoURLScheme(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	_, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: modsapi.RunSubmitRequest{
			RepoURL:   domaintypes.RepoURL("http://example.com/repo.git"),
			BaseRef:   domaintypes.GitRef("main"),
			TargetRef: domaintypes.GitRef("feature"),
		},
	}).Run(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repo URL scheme")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Fatalf("expected error to mention repo_url, got %q", err.Error())
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
			runID := domaintypes.NewRunID()

			// SSE server emits a terminal run event (RunSummary directly, no wrapper).
			sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				runSummary := modsapi.RunSummary{RunID: runID, State: tt.terminalState}
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
			state, err := (EventsCommand{Client: cli, BaseURL: base, RunID: runID}).Run(context.Background())
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
	runID := domaintypes.NewRunID()
	runIDStr := runID.String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad req"}`))
	})
	mux.HandleFunc("/v1/runs/"+runIDStr+"/status", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/v1/runs/"+runIDStr+"/cancel", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusTeapot) })
	mux.HandleFunc("/v1/mods/t", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	// Submit
	if _, err := (SubmitCommand{
		Client:  srv.Client(),
		BaseURL: base,
		Request: modsapi.RunSubmitRequest{
			RepoURL:   domaintypes.RepoURL("https://example.com/repo.git"),
			BaseRef:   domaintypes.GitRef("main"),
			TargetRef: domaintypes.GitRef("feature"),
			Spec:      []byte("{}"),
		},
	}).Run(context.Background()); err == nil {
		t.Fatal("expected submit error")
	}
	// Cancel
	if err := (runs.CancelCommand{Client: srv.Client(), BaseURL: base, RunID: runID}).Run(context.Background()); err == nil {
		t.Fatal("expected cancel error")
	}
}

func TestSimplePrinterFormats(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	var b bytes.Buffer
	p := SimplePrinter{out: &b}
	p.Run(modsapi.RunSummary{RunID: runID, State: modsapi.RunStateRunning})
	p.Stage(modsapi.StageStatus{State: modsapi.StageStateFailed, Attempts: 2, CurrentJobID: jobID, LastError: "boom"})
	if b.Len() == 0 {
		t.Fatalf("expected printer output")
	}
}

// TestEventsCommandWithLogPrinter verifies that EventsCommand renders log events
// using the shared LogPrinter when configured (unified log streaming for mod run --follow).
func TestEventsCommandWithLogPrinter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buildEvents func(runID, nodeID, jobID string) []string
		wantLog     string // expected substring in log output
		wantFinal   modsapi.RunState
		wantNodeID  bool // whether node= context should appear
	}{
		{
			name: "log event with enriched fields",
			buildEvents: func(runID, nodeID, jobID string) []string {
				return []string{
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"running\"}\n\n",
					"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:00Z\",\"stream\":\"stdout\",\"line\":\"Build started\",\"node_id\":\"" + nodeID + "\",\"job_id\":\"" + jobID + "\",\"job_type\":\"mod\",\"next_id\":100}\n\n",
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"succeeded\"}\n\n",
				}
			},
			wantLog:    "Build started",
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: true,
		},
		{
			name: "log event without enriched fields",
			buildEvents: func(runID, _ string, _ string) []string {
				return []string{
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"running\"}\n\n",
					"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:01Z\",\"stream\":\"stderr\",\"line\":\"Warning\"}\n\n",
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"succeeded\"}\n\n",
				}
			},
			wantLog:    "Warning",
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: false,
		},
		{
			name: "retention event recorded",
			buildEvents: func(runID, _ string, _ string) []string {
				return []string{
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"running\"}\n\n",
					"event: retention\ndata: {\"retained\":true,\"ttl\":\"24h\",\"expires_at\":\"2025-10-23T10:00:00Z\",\"bundle_cid\":\"bafy-bundle\"}\n\n",
					"event: run\ndata: {\"run_id\":\"" + runID + "\",\"state\":\"succeeded\"}\n\n",
				}
			},
			wantLog:    "retained", // retention summary is printed
			wantFinal:  modsapi.RunStateSucceeded,
			wantNodeID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runID := domaintypes.NewRunID()
			nodeID := domaintypes.NewNodeKey()
			jobID := domaintypes.NewJobID()

			sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fl, ok := w.(http.Flusher)
				if !ok {
					t.Fatal("no flusher")
				}
				fl.Flush()

				for _, evt := range tt.buildEvents(runID.String(), nodeID, jobID.String()) {
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
				RunID:      runID,
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

// TestEventsCommandWithoutLogPrinter verifies that when LogPrinter is nil,
// EventsCommand still renders log events using a default structured printer.
func TestEventsCommandWithoutLogPrinter(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		fl.Flush()

		// Send run, log, and run events.
		events := []string{
			"event: run\ndata: {\"run_id\":\"" + runID.String() + "\",\"state\":\"running\"}\n\n",
			"event: log\ndata: {\"timestamp\":\"2025-10-22T10:00:00Z\",\"stream\":\"stdout\",\"line\":\"Should be printed\"}\n\n",
			"event: run\ndata: {\"run_id\":\"" + runID.String() + "\",\"state\":\"succeeded\"}\n\n",
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
		RunID:      runID,
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
	// Log message should appear even when LogPrinter is nil.
	if !bytes.Contains([]byte(out), []byte("Should be printed")) {
		t.Errorf("log message should appear when LogPrinter is nil, got: %s", out)
	}
	// Ticket state should still appear via SimplePrinter.
	if !bytes.Contains([]byte(out), []byte(runID.String())) {
		t.Errorf("run ID should appear in output, got: %s", out)
	}
}
