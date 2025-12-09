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
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Format controls log rendering style. Re-exported from internal/cli/logs
// for backward compatibility with existing callers.
type Format = logs.Format

const (
	// FormatStructured includes timestamp, stream labels, and execution context.
	FormatStructured Format = logs.FormatStructured
	// FormatRaw prints log lines as-is (message only).
	FormatRaw Format = logs.FormatRaw
)

// ErrInvalidFormat indicates an unsupported format value.
var ErrInvalidFormat = errors.New("mods: invalid format")

// LogsCommand streams Mods logs over SSE.
type LogsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Format  Format
	Output  io.Writer
}

// Run executes the streaming command.
func (c LogsCommand) Run(ctx context.Context) error {
	format := c.Format
	if format == "" {
		format = FormatStructured
	}
	if format != FormatStructured && format != FormatRaw {
		return ErrInvalidFormat
	}
	if c.RunID.IsZero() {
		return errors.New("mods: run id required")
	}
	if c.BaseURL == nil {
		return errors.New("mods: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(c.RunID.String()), "events")
	if err != nil {
		return fmt.Errorf("mods: build endpoint: %w", err)
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
			var payload logs.LogRecord
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("mods: decode log event: %w", err)
			}
			printer.PrintLog(payload)
		case "retention":
			var hint logs.RetentionHint
			if err := json.Unmarshal(evt.Data, &hint); err != nil {
				return fmt.Errorf("mods: decode retention event: %w", err)
			}
			printer.RecordRetention(hint)
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown event types
		}
		return nil
	}

	if err := c.Client.Stream(ctx, endpoint, handler); err != nil {
		return err
	}
	printer.PrintRetentionSummary()
	return nil
}
