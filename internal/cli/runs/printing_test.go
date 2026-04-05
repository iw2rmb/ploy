package runs

import (
	"bytes"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

func TestPrintLogFormats(t *testing.T) {
	t.Parallel()

	rec := logstream.LogRecord{Timestamp: "2025-01-01T00:00:00Z", Stream: "stderr", Line: "hello\n"}

	tests := []struct {
		name   string
		format logs.Format
		rec    logstream.LogRecord
		want   string
	}{
		{"raw", logs.FormatRaw, rec, "hello\n"},
		{"structured", logs.FormatStructured, rec, "2025-01-01T00:00:00Z stderr hello\n"},
		{"structured defaults", logs.FormatStructured, logstream.LogRecord{Line: "hi\r\n"}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			printer := logs.NewPrinter(tc.format, &b)
			printer.PrintLog(tc.rec)
			got := b.String()
			if tc.want == "" {
				if got == "" {
					t.Fatalf("expected non-empty output for %q", tc.name)
				}
			} else if got != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestPrintRetentionSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hint      *logstream.RetentionHint
		wantEmpty bool
	}{
		{"no hint", nil, true},
		{"retained all fields", &logstream.RetentionHint{Retained: true, TTL: "24h", Expires: "2025-01-02", Bundle: "cid"}, false},
		{"retained ttl only", &logstream.RetentionHint{Retained: true, TTL: "24h"}, false},
		{"retained minimal", &logstream.RetentionHint{Retained: true}, false},
		{"not retained", &logstream.RetentionHint{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			printer := logs.NewPrinter(logs.FormatStructured, &b)
			if tc.hint != nil {
				printer.RecordRetention(*tc.hint)
			}
			printer.PrintRetentionSummary()
			if tc.wantEmpty && b.Len() != 0 {
				t.Fatalf("expected no output, got %q", b.String())
			}
			if !tc.wantEmpty && b.Len() == 0 {
				t.Fatalf("expected output for %q", tc.name)
			}
		})
	}
}

