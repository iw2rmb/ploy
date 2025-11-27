package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestInspectAndArtifactsCommands(t *testing.T) {
	ticket := modsapi.TicketSummary{
		TicketID: domaintypes.TicketID("t1"),
		State:    modsapi.TicketStateSucceeded,
		Stages: map[string]modsapi.StageStatus{
			"build": {StageID: domaintypes.StageID("build"), Artifacts: map[string]string{"bin": "cid1"}},
			"test":  {StageID: domaintypes.StageID("test")},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: ticket})
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	// Inspect prints one-line summary.
	var out bytes.Buffer
	if err := (InspectCommand{Client: srv.Client(), BaseURL: base, Ticket: "t1", Output: &out}).Run(context.Background()); err != nil {
		t.Fatalf("inspect run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected inspect to write output")
	}

	// Artifacts prints per-stage artifacts.
	out.Reset()
	if err := (ArtifactsCommand{Client: srv.Client(), BaseURL: base, Ticket: "t1", Output: &out}).Run(context.Background()); err != nil {
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
		ticket       modsapi.TicketSummary
		wantContains string
	}{
		{
			name: "gate summary present",
			ticket: modsapi.TicketSummary{
				TicketID: domaintypes.TicketID("t-gate-1"),
				State:    modsapi.TicketStateSucceeded,
				Metadata: map[string]string{
					"gate_summary": "passed duration=1234ms",
				},
			},
			wantContains: "Gate: passed duration=1234ms",
		},
		{
			name: "gate summary and MR URL",
			ticket: modsapi.TicketSummary{
				TicketID: domaintypes.TicketID("t-gate-2"),
				State:    modsapi.TicketStateSucceeded,
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
			ticket: modsapi.TicketSummary{
				TicketID: domaintypes.TicketID("t-gate-final-failed"),
				State:    modsapi.TicketStateFailed,
				Metadata: map[string]string{
					"gate_summary": "failed final-gate duration=2345ms",
				},
			},
			wantContains: "Gate: failed final-gate duration=2345ms",
		},
		{
			name: "no gate summary",
			ticket: modsapi.TicketSummary{
				TicketID: domaintypes.TicketID("t-no-gate"),
				State:    modsapi.TicketStateSucceeded,
				Metadata: map[string]string{},
			},
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: tt.ticket})
			}))
			defer srv.Close()
			base, _ := url.Parse(srv.URL)

			var out bytes.Buffer
			cmd := InspectCommand{
				Client:  srv.Client(),
				BaseURL: base,
				Ticket:  string(tt.ticket.TicketID),
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
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: domaintypes.TicketID("t2"), State: modsapi.TicketStatePending}})
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
	sum, err := (SubmitCommand{Client: srv.Client(), BaseURL: base, Request: modsapi.TicketSubmitRequest{TicketID: domaintypes.TicketID("t2")}}).Run(context.Background())
	if err != nil || string(sum.TicketID) != "t2" {
		t.Fatalf("submit err=%v ticket=%+v", err, sum)
	}
	// Cancel
	if err := (CancelCommand{Client: srv.Client(), BaseURL: base, Ticket: "t2"}).Run(context.Background()); err != nil {
		t.Fatalf("cancel err=%v", err)
	}
	// Resume
	if err := (ResumeCommand{Client: srv.Client(), BaseURL: base, Ticket: "t2"}).Run(context.Background()); err != nil {
		t.Fatalf("resume err=%v", err)
	}
}

func TestEventsCommandStreamsToTerminal(t *testing.T) {
	tests := []struct {
		name          string
		terminalState modsapi.TicketState
	}{
		{"succeeded", modsapi.TicketStateSucceeded},
		{"cancelled", modsapi.TicketStateCancelled},
		{"failed", modsapi.TicketStateFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SSE server emits a terminal ticket event.
			sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				evt := struct {
					Ticket modsapi.TicketSummary `json:"ticket"`
				}{Ticket: modsapi.TicketSummary{TicketID: domaintypes.TicketID("t3"), State: tt.terminalState}}
				b, _ := json.Marshal(evt.Ticket)
				_, _ = w.Write([]byte("event: ticket\n"))
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
			state, err := (EventsCommand{Client: cli, BaseURL: base, Ticket: "t3"}).Run(context.Background())
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
	if err := (CancelCommand{Client: srv.Client(), BaseURL: base, Ticket: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected cancel error")
	}
	// Resume
	if err := (ResumeCommand{Client: srv.Client(), BaseURL: base, Ticket: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected resume error")
	}
	// Inspect
	if err := (InspectCommand{Client: srv.Client(), BaseURL: base, Ticket: "t"}).Run(context.Background()); err == nil {
		t.Fatal("expected inspect error")
	}
}

func TestSimplePrinterFormats(t *testing.T) {
	var b bytes.Buffer
	p := SimplePrinter{out: &b}
	p.Ticket(modsapi.TicketSummary{TicketID: domaintypes.TicketID("t1"), State: modsapi.TicketStateRunning})
	p.Stage(modsapi.StageStatus{StageID: domaintypes.StageID("build"), State: modsapi.StageStateFailed, Attempts: 2, CurrentJobID: modsapi.JobID("j1"), LastError: "boom"})
	if b.Len() == 0 {
		t.Fatalf("expected printer output")
	}
}
