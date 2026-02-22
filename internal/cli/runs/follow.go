package runs

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
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// ErrInvalidFormat signals an unsupported format.
var ErrInvalidFormat = errors.New("jobs: invalid format")

// FollowCommand tails a job's logs via SSE.
type FollowCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	JobID   domaintypes.JobID
	Format  logs.Format // Use canonical logs.Format directly.
	Output  io.Writer
}

// Run executes the follow command.
func (c FollowCommand) Run(ctx context.Context) error {
	format := c.Format
	if format == "" {
		format = logs.FormatStructured
	}
	// Validate format against canonical logs.Format constants.
	if format != logs.FormatStructured && format != logs.FormatRaw {
		return ErrInvalidFormat
	}
	if c.JobID.IsZero() {
		return errors.New("jobs: job id required")
	}
	if c.BaseURL == nil {
		return errors.New("jobs: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(c.JobID.String()), "events")
	if err != nil {
		return fmt.Errorf("jobs: build endpoint: %w", err)
	}

	// Use the shared log printer for consistent formatting across CLI commands.
	printer := logs.NewPrinter(format, writer)

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "", "log":
			if len(evt.Data) == 0 {
				return nil
			}
			// Decode into the shared LogRecord type which supports enriched fields.
			var payload logstream.LogRecord
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("jobs: decode log event: %w", err)
			}
			printer.PrintLog(payload)
		case "retention":
			if len(evt.Data) == 0 {
				return nil
			}
			var hint logstream.RetentionHint
			if err := json.Unmarshal(evt.Data, &hint); err != nil {
				return fmt.Errorf("jobs: decode retention event: %w", err)
			}
			printer.RecordRetention(hint)
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown events
		}
		return nil
	}

	if err := c.Client.Stream(ctx, endpoint, handler); err != nil {
		return err
	}
	printer.PrintRetentionSummary()
	return nil
}
