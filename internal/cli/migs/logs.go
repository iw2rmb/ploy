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

// LogsCommand streams run lifecycle events for a single Migs run over SSE.
// The run stream carries only lifecycle frames (run, stage, done); container
// log frames are served via the job-scoped endpoint.
type LogsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Output  io.Writer
}

// Run executes the streaming command, printing lifecycle updates until done.
func (c LogsCommand) Run(ctx context.Context) error {
	if c.RunID.IsZero() {
		return errors.New("migs: run id required")
	}
	if c.BaseURL == nil {
		return errors.New("migs: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "logs")

	printer := &SimplePrinter{out: writer}

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			var t migsapi.RunSummary
			if err := json.Unmarshal(evt.Data, &t); err != nil {
				return fmt.Errorf("migs: decode run event: %w", err)
			}
			printer.Run(t)
			if isTerminalRunState(t.State) {
				return stream.ErrDone
			}
		case "stage":
			var payload struct {
				RunID domaintypes.RunID   `json:"run_id"`
				Stage migsapi.StageStatus `json:"stage"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("migs: decode stage event: %w", err)
			}
			printer.Stage(payload.Stage)
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown event types
		}
		return nil
	}

	return c.Client.Stream(ctx, endpoint.String(), handler)
}
