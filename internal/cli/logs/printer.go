// Package logs provides a shared log printer for CLI commands that consume
// enriched log events from the job-scoped SSE stream. The `ploy job follow`
// command delegates to this printer to ensure consistent formatting across
// all log-consuming commands.
//
// This package uses the canonical stream.LogRecord type from internal/stream
// to ensure a single source of truth for log payload structures across the
// server publish path and CLI decode path. This eliminates duplicate struct
// definitions and prevents drift between server and CLI.
package logs

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// Format controls log rendering style.
type Format string

const (
	// FormatStructured includes timestamp, stream labels, and execution context.
	FormatStructured Format = "structured"
	// FormatRaw prints log lines as-is (message only).
	FormatRaw Format = "raw"
)

// Printer formats and writes log records to an output stream.
// Thread-safe for use in SSE event handlers.
type Printer struct {
	mu        sync.Mutex
	format    Format
	out       io.Writer
	retention *logstream.RetentionHint
}

// NewPrinter creates a log printer with the given format and output writer.
// If format is empty, defaults to FormatStructured.
// If out is nil, logs are discarded silently.
func NewPrinter(format Format, out io.Writer) *Printer {
	if format == "" {
		format = FormatStructured
	}
	if out == nil {
		out = io.Discard
	}
	return &Printer{
		format: format,
		out:    out,
	}
}

// PrintLog formats and writes a single log record.
//
// Structured format (when enriched fields are present):
//
//	2025-10-22T10:00:00Z stdout node=<node_id> job_type=<job_type> step=<next_id> job=<job_id> Step started
//
// Structured format (basic, no enriched fields):
//
//	2025-10-22T10:00:00Z stdout Step started
//
// Raw format (message only):
//
//	Step started
func (p *Printer) PrintLog(rec logstream.LogRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Strip trailing newlines to avoid double line breaks.
	line := strings.TrimRight(rec.Line, "\r\n")

	switch p.format {
	case FormatRaw:
		// Raw mode: message only, no timestamp or stream labels.
		_, _ = fmt.Fprintf(p.out, "%s\n", line)
	default:
		// Structured mode: timestamp stream [context...] message
		timestamp := strings.TrimSpace(rec.Timestamp)
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}
		stream := strings.TrimSpace(rec.Stream)
		if stream == "" {
			stream = "stdout"
		}

		// Build context string from enriched fields (only include non-empty).
		// Order: node, job_type, job — most general to most specific.
		// Use domain type's IsZero method to check for empty values.
		var ctx strings.Builder
		if !rec.NodeID.IsZero() {
			ctx.WriteString("node=")
			ctx.WriteString(rec.NodeID.String())
		}
		// JobType uses domaintypes.JobType under the hood; use IsZero() consistently.
		if !rec.JobType.IsZero() {
			if ctx.Len() > 0 {
				ctx.WriteByte(' ')
			}
			ctx.WriteString("job_type=")
			ctx.WriteString(rec.JobType.String())
		}
		if !rec.JobID.IsZero() {
			if ctx.Len() > 0 {
				ctx.WriteByte(' ')
			}
			ctx.WriteString("job=")
			ctx.WriteString(rec.JobID.String())
		}

		// Format: "timestamp stream [context ] line"
		// Include context prefix only if we have any enriched fields.
		if ctx.Len() > 0 {
			_, _ = fmt.Fprintf(p.out, "%s %s %s %s\n", timestamp, stream, ctx.String(), line)
		} else {
			_, _ = fmt.Fprintf(p.out, "%s %s %s\n", timestamp, stream, line)
		}
	}
}

// RecordRetention stores a retention hint to be printed at stream completion.
// Only the last recorded hint is printed (calls overwrite previous hints).
func (p *Printer) RecordRetention(hint logstream.RetentionHint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	copy := hint
	p.retention = &copy
}

// PrintRetentionSummary outputs the stored retention hint, if any.
// Call this after all log events have been processed.
func (p *Printer) PrintRetentionSummary() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.retention == nil {
		return
	}
	ret := *p.retention
	ttl := strings.TrimSpace(ret.TTL)
	expires := strings.TrimSpace(ret.Expires)
	bundle := strings.TrimSpace(string(ret.Bundle))

	// Format retention summary with available fields.
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

// Format returns the printer's configured format.
func (p *Printer) Format() Format {
	return p.format
}
