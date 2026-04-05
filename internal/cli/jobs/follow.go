package jobs

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

// FollowCommand streams job logs from the job SSE endpoint.
type FollowCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	JobID   domaintypes.JobID
	Format  logs.Format
	Output  io.Writer
}

// Run executes the streaming command against /v1/jobs/{job_id}/logs.
func (c FollowCommand) Run(ctx context.Context) error {
	format := c.Format
	if format == "" {
		format = logs.FormatStructured
	}
	if format != logs.FormatStructured && format != logs.FormatRaw {
		return ErrInvalidFormat
	}
	if c.JobID.IsZero() {
		return errors.New("job follow: job id required")
	}
	if c.BaseURL == nil {
		return errors.New("job follow: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint := c.BaseURL.JoinPath("v1", "jobs", c.JobID.String(), "logs")

	printer := logs.NewPrinter(format, writer)

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "", "log":
			if len(evt.Data) == 0 {
				return nil
			}
			var payload logstream.LogRecord
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("job follow: decode log event: %w", err)
			}
			printer.PrintLog(payload)
		case "retention":
			var hint logstream.RetentionHint
			if err := json.Unmarshal(evt.Data, &hint); err != nil {
				return fmt.Errorf("job follow: decode retention event: %w", err)
			}
			printer.RecordRetention(hint)
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown event types
		}
		return nil
	}

	if err := c.Client.Stream(ctx, endpoint.String(), handler); err != nil {
		return err
	}
	printer.PrintRetentionSummary()
	return nil
}

// ErrInvalidFormat indicates an unsupported format value.
var ErrInvalidFormat = errors.New("job follow: invalid format")
