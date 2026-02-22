package runs

import (
	"bytes"
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// TestPrintLogFormats verifies log formatting via the shared printer.
func TestPrintLogFormats(t *testing.T) {
	t.Parallel()

	rec := logstream.LogRecord{Timestamp: "2025-01-01T00:00:00Z", Stream: "stderr", Line: "hello\n"}

	// Raw format: message only — use canonical logs.FormatRaw directly.
	var b bytes.Buffer
	printer := logs.NewPrinter(logs.FormatRaw, &b)
	printer.PrintLog(rec)
	if got := b.String(); got != "hello\n" {
		t.Fatalf("raw got %q, want %q", got, "hello\n")
	}

	// Structured format: timestamp stream message — use canonical logs.FormatStructured.
	b.Reset()
	printer = logs.NewPrinter(logs.FormatStructured, &b)
	printer.PrintLog(rec)
	if got := b.String(); got != "2025-01-01T00:00:00Z stderr hello\n" {
		t.Fatalf("structured got %q", got)
	}

	// Missing timestamp/stream falls back to defaults and trims CRLF.
	b.Reset()
	printer.PrintLog(logstream.LogRecord{Line: "hi\r\n"})
	if got := b.String(); got == "" {
		t.Fatalf("expected non-empty structured default output")
	}
}

// TestPrintRetentionSummary verifies retention hint formatting via the shared printer.
func TestPrintRetentionSummary(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer
	printer := logs.NewPrinter(logs.FormatStructured, &b)

	// No hint recorded => no output
	printer.PrintRetentionSummary()
	if b.Len() != 0 {
		t.Fatalf("expected no output when no retention recorded")
	}

	// Retained with all fields
	b.Reset()
	printer = logs.NewPrinter(logs.FormatStructured, &b)
	printer.RecordRetention(logstream.RetentionHint{Retained: true, TTL: "24h", Expires: "2025-01-02", Bundle: "cid"})
	printer.PrintRetentionSummary()
	if b.Len() == 0 {
		t.Fatalf("expected output for retained with ttl+expires")
	}

	// Retained with ttl only
	b.Reset()
	printer = logs.NewPrinter(logs.FormatStructured, &b)
	printer.RecordRetention(logstream.RetentionHint{Retained: true, TTL: "24h"})
	printer.PrintRetentionSummary()
	if b.Len() == 0 {
		t.Fatalf("expected output for retained with ttl")
	}

	// Retained minimal
	b.Reset()
	printer = logs.NewPrinter(logs.FormatStructured, &b)
	printer.RecordRetention(logstream.RetentionHint{Retained: true})
	printer.PrintRetentionSummary()
	if b.Len() == 0 {
		t.Fatalf("expected output for retained minimal")
	}

	// Not retained
	b.Reset()
	printer = logs.NewPrinter(logs.FormatStructured, &b)
	printer.RecordRetention(logstream.RetentionHint{})
	printer.PrintRetentionSummary()
	if b.Len() == 0 {
		t.Fatalf("expected output for not retained")
	}
}

// TestFollowCommandInvalidFormat verifies format validation still works.
func TestFollowCommandInvalidFormat(t *testing.T) {
	t.Parallel()
	err := (FollowCommand{Format: "bad"}).Run(context.TODO())
	if err == nil || err != ErrInvalidFormat {
		t.Fatalf("expected ErrInvalidFormat, got %v", err)
	}
}
