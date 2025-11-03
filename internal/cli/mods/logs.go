package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/stream"
)

// Format controls log rendering style.
type Format string

const (
	// FormatStructured includes timestamp and stream labels.
	FormatStructured Format = "structured"
	// FormatRaw prints log lines as-is.
	FormatRaw Format = "raw"
)

// ErrInvalidFormat indicates an unsupported format value.
var ErrInvalidFormat = errors.New("mods: invalid format")

// LogsCommand streams Mods logs over SSE.
type LogsCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	Ticket  string
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
	if strings.TrimSpace(c.Ticket) == "" {
		return errors.New("mods: ticket required")
	}
	if c.BaseURL == nil {
		return errors.New("mods: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(strings.TrimSpace(c.Ticket)), "events")
	if err != nil {
		return fmt.Errorf("mods: build endpoint: %w", err)
	}

	printer := &logPrinter{
		format: format,
		out:    writer,
	}

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "", "log":
			var payload logEvent
			if len(evt.Data) == 0 {
				return nil
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("mods: decode log event: %w", err)
			}
			printer.printLog(payload)
		case "retention":
			var hint retentionEvent
			if err := json.Unmarshal(evt.Data, &hint); err != nil {
				return fmt.Errorf("mods: decode retention event: %w", err)
			}
			printer.recordRetention(hint)
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
	printer.printRetentionSummary()
	return nil
}

type logEvent struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
}

type retentionEvent struct {
	Retained bool   `json:"retained"`
	TTL      string `json:"ttl"`
	Expires  string `json:"expires_at"`
	Bundle   string `json:"bundle_cid"`
}

type logPrinter struct {
	format    Format
	out       io.Writer
	retention *retentionEvent
}

func (p *logPrinter) printLog(evt logEvent) {
	line := strings.TrimRight(evt.Line, "\r\n")
	switch p.format {
	case FormatRaw:
		_, _ = fmt.Fprintf(p.out, "%s\n", line)
	default:
		timestamp := strings.TrimSpace(evt.Timestamp)
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}
		stream := strings.TrimSpace(evt.Stream)
		if stream == "" {
			stream = "stdout"
		}
		_, _ = fmt.Fprintf(p.out, "%s %s %s\n", timestamp, stream, line)
	}
}

func (p *logPrinter) recordRetention(evt retentionEvent) {
	copy := evt
	p.retention = &copy
}

func (p *logPrinter) printRetentionSummary() {
	if p.retention == nil {
		return
	}
	ret := *p.retention
	ttl := strings.TrimSpace(ret.TTL)
	expires := strings.TrimSpace(ret.Expires)
	bundle := strings.TrimSpace(ret.Bundle)

	switch {
	case ret.Retained && ttl != "" && expires != "":
		_, _ = fmt.Fprintf(p.out, "Retention: retained ttl=%s expires=%s cid=%s\n", ttl, expires, bundle)
	case ret.Retained && ttl != "":
		_, _ = fmt.Fprintf(p.out, "Retention: retained ttl=%s cid=%s\n", ttl, bundle)
	case ret.Retained:
		_, _ = fmt.Fprintf(p.out, "Retention: retained cid=%s\n", bundle)
	default:
		_, _ = fmt.Fprintln(p.out, "Retention: not retained (bundle expires per default policy)")
	}
}
