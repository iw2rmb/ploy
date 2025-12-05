package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// EventsPrinter renders ticket and stage updates.
type EventsPrinter interface {
	Ticket(modsapi.TicketSummary)
	Stage(stage modsapi.StageStatus)
}

// SimplePrinter prints a short human-readable summary.
type SimplePrinter struct{ out io.Writer }

func (p SimplePrinter) Ticket(t modsapi.TicketSummary) {
	_, _ = fmt.Fprintf(p.out, "Ticket %s: %s\n", strings.TrimSpace(string(t.TicketID)), strings.ToLower(string(t.State)))
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
type EventsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	Ticket  string
	Output  io.Writer
	Printer EventsPrinter
}

// Run consumes "ticket" and "stage" SSE events from /v1/mods/{id}/events.
// Unknown event types are ignored so the CLI remains forward compatible and it
// returns the final ticket state.
func (c EventsCommand) Run(ctx context.Context) (modsapi.TicketState, error) {
	if c.Client.HTTPClient == nil {
		return "", errors.New("mods events: http client required")
	}
	if c.BaseURL == nil {
		return "", errors.New("mods events: base url required")
	}
	ticket := strings.TrimSpace(c.Ticket)
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
	var final modsapi.TicketState
	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "ticket":
			var t modsapi.TicketSummary
			if err := json.Unmarshal(evt.Data, &t); err != nil {
				return fmt.Errorf("mods events: decode ticket: %w", err)
			}
			printer.Ticket(t)
			if isTerminalTicketState(t.State) {
				final = t.State
				return stream.ErrDone
			}
		case "stage":
			var payload struct {
				TicketID string              `json:"ticket_id"`
				Stage    modsapi.StageStatus `json:"stage"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("mods events: decode stage: %w", err)
			}
			printer.Stage(payload.Stage)
		default:
			// ignore unknown event types
		}
		return nil
	}
	if err := c.Client.Stream(ctx, endpoint, handler); err != nil {
		return "", err
	}
	return final, nil
}

func isTerminalTicketState(s modsapi.TicketState) bool {
	switch s {
	case modsapi.TicketStateSucceeded, modsapi.TicketStateFailed, modsapi.TicketStateCancelled:
		return true
	default:
		return false
	}
}
