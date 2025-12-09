// Package logs provides a shared log printer for CLI commands that consume
// enriched log events from the Mods SSE stream. Both `ploy mods logs` and
// `ploy runs follow` delegate to this printer to ensure consistent formatting
// across all log-consuming commands.
package logs

import (
	"fmt"
	"io"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Format controls log rendering style.
type Format string

const (
	// FormatStructured includes timestamp, stream labels, and execution context.
	FormatStructured Format = "structured"
	// FormatRaw prints log lines as-is (message only).
	FormatRaw Format = "raw"
)

// LogRecord represents an enriched log event from the Mods SSE stream.
// Fields mirror internal/stream.LogRecord for consistency; optional fields
// (NodeID, JobID, ModType, StepIndex) provide execution context when available.
// Uses domain types (NodeID, JobID) for type-safe identification.
type LogRecord struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`

	// NodeID identifies the execution node that produced this log line (NanoID-backed).
	// Empty when the source is not node-bound.
	NodeID domaintypes.NodeID `json:"node_id,omitempty"`

	// JobID is the ID of the job that produced this log line (KSUID-backed).
	// Empty for events not tied to a specific job.
	JobID domaintypes.JobID `json:"job_id,omitempty"`

	// ModType indicates the Mods step type (e.g., "pre_gate", "mod", "post_gate", "heal", "re_gate").
	// Empty when not applicable or unknown.
	ModType string `json:"mod_type,omitempty"`

	// StepIndex mirrors jobs.step_index and is used to order steps within a Mods run.
	// Typical values are float-style indices (1000, 1500, 2000, 3000); zero when omitted.
	StepIndex int `json:"step_index,omitempty"`
}

// RetentionHint carries retention metadata emitted at stream completion.
type RetentionHint struct {
	Retained bool   `json:"retained"`
	TTL      string `json:"ttl"`
	Expires  string `json:"expires_at"`
	Bundle   string `json:"bundle_cid"`
}

// Printer formats and writes log records to an output stream.
// Thread-safe for use in SSE event handlers.
type Printer struct {
	format    Format
	out       io.Writer
	retention *RetentionHint
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
//	2025-10-22T10:00:00Z stdout node=<node_id> mod=<mod_type> step=<step_index> job=<job_id> Step started
//
// Structured format (basic, no enriched fields):
//
//	2025-10-22T10:00:00Z stdout Step started
//
// Raw format (message only):
//
//	Step started
func (p *Printer) PrintLog(rec LogRecord) {
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
		// Order: node, mod, step, job — most general to most specific.
		// Use domain type's IsZero method to check for empty values.
		var ctx strings.Builder
		if !rec.NodeID.IsZero() {
			ctx.WriteString("node=")
			ctx.WriteString(rec.NodeID.String())
		}
		if rec.ModType != "" {
			if ctx.Len() > 0 {
				ctx.WriteByte(' ')
			}
			ctx.WriteString("mod=")
			ctx.WriteString(rec.ModType)
		}
		// StepIndex: include if > 0 (zero typically means "not set").
		if rec.StepIndex > 0 {
			if ctx.Len() > 0 {
				ctx.WriteByte(' ')
			}
			fmt.Fprintf(&ctx, "step=%d", rec.StepIndex)
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
func (p *Printer) RecordRetention(hint RetentionHint) {
	copy := hint
	p.retention = &copy
}

// PrintRetentionSummary outputs the stored retention hint, if any.
// Call this after all log events have been processed.
func (p *Printer) PrintRetentionSummary() {
	if p.retention == nil {
		return
	}
	ret := *p.retention
	ttl := strings.TrimSpace(ret.TTL)
	expires := strings.TrimSpace(ret.Expires)
	bundle := strings.TrimSpace(ret.Bundle)

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
