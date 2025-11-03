package runs

import (
	"bytes"
	"context"
	"testing"
)

func TestPrintLogFormats(t *testing.T) {
	evt := logEvent{Timestamp: "2025-01-01T00:00:00Z", Stream: "stderr", Line: "hello\n"}
	var b bytes.Buffer
	printLog(&b, FormatRaw, evt)
	if got := b.String(); got != "hello\n" {
		t.Fatalf("raw got %q", got)
	}
	b.Reset()
	printLog(&b, FormatStructured, evt)
	if got := b.String(); got != "2025-01-01T00:00:00Z stderr hello\n" {
		t.Fatalf("structured got %q", got)
	}
	// Missing timestamp/stream falls back to defaults and trims CRLF.
	b.Reset()
	printLog(&b, FormatStructured, logEvent{Line: "hi\r\n"})
	if got := b.String(); got == "" {
		t.Fatalf("expected non-empty structured default output")
	}
}

func TestPrintRetentionSummary(t *testing.T) {
	var b bytes.Buffer
	// Nil => no output
	printRetentionSummary(&b, nil)
	if b.Len() != 0 {
		t.Fatalf("expected no output for nil event")
	}
	// Retained with all fields
	b.Reset()
	printRetentionSummary(&b, &retentionEvent{Retained: true, TTL: "24h", Expires: "2025-01-02", Bundle: "cid"})
	if b.Len() == 0 {
		t.Fatalf("expected output for retained with ttl+expires")
	}
	// Retained with ttl only
	b.Reset()
	printRetentionSummary(&b, &retentionEvent{Retained: true, TTL: "24h"})
	if b.Len() == 0 {
		t.Fatalf("expected output for retained with ttl")
	}
	// Retained minimal
	b.Reset()
	printRetentionSummary(&b, &retentionEvent{Retained: true})
	if b.Len() == 0 {
		t.Fatalf("expected output for retained minimal")
	}
	// Not retained
	b.Reset()
	printRetentionSummary(&b, &retentionEvent{})
	if b.Len() == 0 {
		t.Fatalf("expected output for not retained")
	}
}

func TestFollowCommandInvalidFormat(t *testing.T) {
	err := (FollowCommand{Format: "bad"}).Run(context.TODO())
	if err == nil || err != ErrInvalidFormat {
		t.Fatalf("expected ErrInvalidFormat, got %v", err)
	}
}
