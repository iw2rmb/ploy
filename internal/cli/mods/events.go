package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// EventsPrinter renders ticket and stage updates.
type EventsPrinter interface {
	Run(modsapi.TicketSummary)
	Stage(stage modsapi.StageStatus)
}

// SimplePrinter prints a short human-readable summary.
type SimplePrinter struct{ out io.Writer }

func (p SimplePrinter) Run(t modsapi.TicketSummary) {
	_, _ = fmt.Fprintf(p.out, "Run %s: %s\n", strings.TrimSpace(string(t.TicketID)), strings.ToLower(string(t.State)))
}
func (p SimplePrinter) Stage(s modsapi.StageStatus) {
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

// EventsCommand streams ticket events until a terminal state is reached.
// When LogPrinter is set, also handles "log" events using the shared log printer
// for unified log output alongside ticket/stage updates (used by `mod run --follow`).
type EventsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	Run     string
	Output  io.Writer
	Printer EventsPrinter

	// LogPrinter is an optional log printer for handling "log" events.
	// When set, enriched log events are rendered using the shared logs.Printer,
	// providing a consistent view for `mod run --follow`. When nil, log events
	// are ignored (backward-compatible with existing behavior).
	LogPrinter *logs.Printer
}

// Run consumes "run", "stage", and optionally "log" SSE events from /v1/mods/{id}/events.
// Unknown event types are ignored so the CLI remains forward compatible. Returns the final
// ticket state. When LogPrinter is set, "log" events are rendered using the shared printer.
func (c EventsCommand) Run(ctx context.Context) (modsapi.RunState, error) {
	if c.Client.HTTPClient == nil {
		return "", errors.New("mods events: http client required")
	}
	if c.BaseURL == nil {
		return "", errors.New("mods events: base url required")
	}
	ticket := strings.TrimSpace(c.Run)
	if ticket == "" {
		return "", errors.New("mods events: ticket required")
	}
	out := c.Output
	if out == nil {
		out = io.Discard
	}
	printer := c.Printer
	if printer == nil {
		printer = SimplePrinter{out: out}
	}
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(ticket), "events")
	if err != nil {
		return "", err
	}
	var final modsapi.RunState
	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			var t modsapi.TicketSummary
			if err := json.Unmarshal(evt.Data, &t); err != nil {
				return fmt.Errorf("mods events: decode ticket: %w", err)
			}
			printer.Run(t)
			if isTerminalRunState(t.State) {
				final = t.State
				return stream.ErrDone
			}
		case "stage":
			var payload struct {
				TicketID string              `json:"run_id"`
				Stage    modsapi.StageStatus `json:"stage"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("mods events: decode stage: %w", err)
			}
			printer.Stage(payload.Stage)
		case "", "log":
			// Handle log events when LogPrinter is configured (for unified log streaming).
			// Empty event type is treated as "log" for backward compatibility with servers
			// that omit the event type field.
			if c.LogPrinter != nil && len(evt.Data) > 0 {
				var rec logs.LogRecord
				if err := json.Unmarshal(evt.Data, &rec); err != nil {
					return fmt.Errorf("mods events: decode log: %w", err)
				}
				c.LogPrinter.PrintLog(rec)
			}
		case "retention":
			// Handle retention hints when LogPrinter is configured.
			// Retention metadata is recorded for summary output at stream completion.
			if c.LogPrinter != nil && len(evt.Data) > 0 {
				var hint logs.RetentionHint
				if err := json.Unmarshal(evt.Data, &hint); err != nil {
					return fmt.Errorf("mods events: decode retention: %w", err)
				}
				c.LogPrinter.RecordRetention(hint)
			}
		default:
			// ignore unknown event types
		}
		return nil
	}
	if err := c.Client.Stream(ctx, endpoint, handler); err != nil {
		return "", err
	}
	// Print retention summary if LogPrinter was configured (for unified log streaming).
	if c.LogPrinter != nil {
		c.LogPrinter.PrintRetentionSummary()
	}
	return final, nil
}

func isTerminalRunState(s modsapi.RunState) bool {
	switch s {
	case modsapi.RunStateSucceeded, modsapi.RunStateFailed, modsapi.RunStateCancelled:
		return true
	default:
		return false
	}
}
