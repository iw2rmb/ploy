package migs

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

// ErrInvalidFormat indicates an unsupported format value.
var ErrInvalidFormat = errors.New("migs: invalid format")

// LogsCommand streams logs for a single Migs run over SSE.
type LogsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Format  logs.Format // Use canonical logs.Format directly.
	Output  io.Writer
}

// Run executes the streaming command.
func (c LogsCommand) Run(ctx context.Context) error {
	format := c.Format
	if format == "" {
		format = logs.FormatStructured
	}
	// Validate format against canonical logs.Format constants.
	if format != logs.FormatStructured && format != logs.FormatRaw {
		return ErrInvalidFormat
	}
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
				return fmt.Errorf("migs: decode log event: %w", err)
			}
			printer.PrintLog(payload)
		case "retention":
			var hint logstream.RetentionHint
			if err := json.Unmarshal(evt.Data, &hint); err != nil {
				return fmt.Errorf("migs: decode retention event: %w", err)
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
