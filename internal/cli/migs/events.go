package migs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// SimplePrinter prints a short human-readable summary of run and stage updates.
type SimplePrinter struct{ out io.Writer }

func (p *SimplePrinter) Run(t migsapi.RunSummary) {
	_, _ = fmt.Fprintf(p.out, "Run %s: %s\n", strings.TrimSpace(string(t.RunID)), strings.ToLower(string(t.State)))
}
func (p *SimplePrinter) Stage(s migsapi.StageStatus) {
	label := strings.TrimSpace(string(s.CurrentJobID))
	if label == "" {
		label = "<stage>"
	}
	line := fmt.Sprintf("  %s -> %s", label, strings.ToLower(string(s.State)))
	if s.Attempts > 0 {
		line += fmt.Sprintf(" attempts=%d", s.Attempts)
	}
	if id := strings.TrimSpace(string(s.CurrentJobID)); id != "" {
		line += fmt.Sprintf(" job=%s", id)
	}
	if msg := strings.TrimSpace(s.LastError); msg != "" {
		line += fmt.Sprintf(" error=%s", msg)
	}
	_, _ = io.WriteString(p.out, line+"\n")
}

// EventsCommand streams run lifecycle events until a terminal state is reached.
// Only run, stage, and done events are consumed from /v1/runs/{id}/logs.
// Container log frames are served via the job-scoped endpoint.
// Uses domain type (RunID) for type-safe identification.
type EventsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID // Run ID (KSUID-backed)
	Output  io.Writer
	Printer *SimplePrinter
}

// Run consumes "run", "stage", and "done" SSE events from /v1/runs/{id}/logs.
// Unknown event types are ignored so the CLI remains forward compatible. Returns the final
// run state.
func (c EventsCommand) Run(ctx context.Context) (migsapi.RunState, error) {
	if c.Client.HTTPClient == nil {
		return "", errors.New("migs events: http client required")
	}
	if c.BaseURL == nil {
		return "", errors.New("migs events: base url required")
	}
	// Use domain type's IsZero method for validation.
	if c.RunID.IsZero() {
		return "", errors.New("migs events: run id required")
	}
	runID := c.RunID.String()
	out := c.Output
	if out == nil {
		out = io.Discard
	}

	// Default run/stage printer.
	printer := c.Printer
	if printer == nil {
		printer = &SimplePrinter{out: out}
	}

	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(runID), "logs")
	if err != nil {
		return "", err
	}
	var final migsapi.RunState
	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			var t migsapi.RunSummary
			if err := json.Unmarshal(evt.Data, &t); err != nil {
				return fmt.Errorf("migs events: decode run: %w", err)
			}
			printer.Run(t)
			if isTerminalRunState(t.State) {
				final = t.State
				return stream.ErrDone
			}
		case "stage":
			var payload struct {
				RunID domaintypes.RunID   `json:"run_id"` // Run ID for the stage event
				Stage migsapi.StageStatus `json:"stage"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("migs events: decode stage: %w", err)
			}
			printer.Stage(payload.Stage)
		default:
			// ignore unknown event types (including done — terminal state
			// is detected from the run event payload)
		}
		return nil
	}
	if err := c.Client.Stream(ctx, endpoint, handler); err != nil {
		return "", err
	}
	return final, nil
}

func isTerminalRunState(s migsapi.RunState) bool {
	switch s {
	case migsapi.RunStateSucceeded, migsapi.RunStateFailed, migsapi.RunStateCancelled:
		return true
	default:
		return false
	}
}
