package logs

import (
	"bytes"
	"testing"

	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// TestPrintLog_StructuredBasic verifies structured output without enriched fields.
func TestPrintLog_StructuredBasic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rec  logstream.LogRecord
		want string
	}{
		{
			name: "stdout with timestamp",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:00Z",
				Stream:    "stdout",
				Line:      "Step started",
			},
			want: "2025-10-22T10:00:00Z stdout Step started\n",
		},
		{
			name: "stderr with timestamp",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:01Z",
				Stream:    "stderr",
				Line:      "warning: slow retry",
			},
			want: "2025-10-22T10:00:01Z stderr warning: slow retry\n",
		},
		{
			name: "empty stream defaults to stdout",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:02Z",
				Stream:    "",
				Line:      "no stream",
			},
			want: "2025-10-22T10:00:02Z stdout no stream\n",
		},
		{
			name: "trailing newline stripped",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:03Z",
				Stream:    "stdout",
				Line:      "trailing newline\n",
			},
			want: "2025-10-22T10:00:03Z stdout trailing newline\n",
		},
		{
			name: "trailing CRLF stripped",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:04Z",
				Stream:    "stdout",
				Line:      "crlf ending\r\n",
			},
			want: "2025-10-22T10:00:04Z stdout crlf ending\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			p := NewPrinter(FormatStructured, buf)
			p.PrintLog(tc.rec)
			if got := buf.String(); got != tc.want {
				t.Errorf("PrintLog() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestPrintLog_StructuredEnriched verifies structured output with enriched fields.
func TestPrintLog_StructuredEnriched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rec  logstream.LogRecord
		want string
	}{
		{
			name: "all enriched fields present",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:00Z",
				Stream:    "stdout",
				Line:      "Step started",
				NodeID:    "node-abc",
				JobType:   "mod",
				StepIndex: 2000,
				JobID:     "job-123",
			},
			want: "2025-10-22T10:00:00Z stdout node=node-abc job_type=mod step=2000 job=job-123 Step started\n",
		},
		{
			name: "node_id only",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:01Z",
				Stream:    "stdout",
				Line:      "partial context",
				NodeID:    "node-xyz",
			},
			want: "2025-10-22T10:00:01Z stdout node=node-xyz partial context\n",
		},
		{
			name: "job_type and job_id only",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:02Z",
				Stream:    "stderr",
				Line:      "gate failure",
				JobType:   "pre_gate",
				JobID:     "job-456",
			},
			want: "2025-10-22T10:00:02Z stderr job_type=pre_gate job=job-456 gate failure\n",
		},
		{
			name: "next_id zero omitted",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:03Z",
				Stream:    "stdout",
				Line:      "step index zero",
				NodeID:    "node-def",
				StepIndex: 0,
			},
			want: "2025-10-22T10:00:03Z stdout node=node-def step index zero\n",
		},
		{
			name: "next_id one included",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:04Z",
				Stream:    "stdout",
				Line:      "step index one",
				StepIndex: 1,
			},
			want: "2025-10-22T10:00:04Z stdout step=1 step index one\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			p := NewPrinter(FormatStructured, buf)
			p.PrintLog(tc.rec)
			if got := buf.String(); got != tc.want {
				t.Errorf("PrintLog() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestPrintLog_Raw verifies raw output (message only).
func TestPrintLog_Raw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rec  logstream.LogRecord
		want string
	}{
		{
			name: "basic line",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:00Z",
				Stream:    "stdout",
				Line:      "ready",
			},
			want: "ready\n",
		},
		{
			name: "enriched fields ignored",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:01Z",
				Stream:    "stderr",
				Line:      "warn",
				NodeID:    "node-ignored",
				JobType:   "hook",
				StepIndex: 5,
				JobID:     "job-ignored",
			},
			want: "warn\n",
		},
		{
			name: "trailing newline stripped",
			rec: logstream.LogRecord{
				Timestamp: "2025-10-22T10:00:02Z",
				Stream:    "stdout",
				Line:      "done\n",
			},
			want: "done\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			p := NewPrinter(FormatRaw, buf)
			p.PrintLog(tc.rec)
			if got := buf.String(); got != tc.want {
				t.Errorf("PrintLog() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestRetentionSummary verifies retention hint formatting.
func TestRetentionSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hint *logstream.RetentionHint
		want string
	}{
		{
			name: "nil hint produces no output",
			hint: nil,
			want: "",
		},
		{
			name: "retained with ttl and expires",
			hint: &logstream.RetentionHint{
				Retained: true,
				TTL:      "72h",
				Expires:  "2025-10-25T10:00:00Z",
				Bundle:   "bafy-bundle",
			},
			want: "Retention: retained ttl=72h expires=2025-10-25T10:00:00Z cid=bafy-bundle\n",
		},
		{
			name: "retained with ttl only",
			hint: &logstream.RetentionHint{
				Retained: true,
				TTL:      "48h",
				Bundle:   "bafy-ttl",
			},
			want: "Retention: retained ttl=48h cid=bafy-ttl\n",
		},
		{
			name: "retained without ttl",
			hint: &logstream.RetentionHint{
				Retained: true,
				Bundle:   "bafy-plain",
			},
			want: "Retention: retained cid=bafy-plain\n",
		},
		{
			name: "not retained",
			hint: &logstream.RetentionHint{
				Retained: false,
			},
			want: "Retention: not retained (bundle expires per default policy)\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}
			p := NewPrinter(FormatStructured, buf)
			if tc.hint != nil {
				p.RecordRetention(*tc.hint)
			}
			p.PrintRetentionSummary()
			if got := buf.String(); got != tc.want {
				t.Errorf("PrintRetentionSummary() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestNewPrinter_Defaults verifies default format and nil output handling.
func TestNewPrinter_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("empty format defaults to structured", func(t *testing.T) {
		t.Parallel()
		p := NewPrinter("", &bytes.Buffer{})
		if got := p.Format(); got != FormatStructured {
			t.Errorf("Format() = %q, want %q", got, FormatStructured)
		}
	})

	t.Run("nil output does not panic", func(t *testing.T) {
		t.Parallel()
		p := NewPrinter(FormatStructured, nil)
		// Should not panic when printing to nil (discarded).
		p.PrintLog(logstream.LogRecord{Line: "test"})
		p.RecordRetention(logstream.RetentionHint{Retained: true})
		p.PrintRetentionSummary()
	})
}

// TestMultipleLogs verifies sequential log printing.
func TestMultipleLogs(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	p := NewPrinter(FormatStructured, buf)

	p.PrintLog(logstream.LogRecord{
		Timestamp: "2025-10-22T10:00:00Z",
		Stream:    "stdout",
		Line:      "Step started",
	})
	p.PrintLog(logstream.LogRecord{
		Timestamp: "2025-10-22T10:00:01Z",
		Stream:    "stderr",
		Line:      "warning: slow retry",
	})
	p.RecordRetention(logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2025-10-25T10:00:00Z",
		Bundle:   "bafy-ret-bundle",
	})
	p.PrintRetentionSummary()

	want := "2025-10-22T10:00:00Z stdout Step started\n" +
		"2025-10-22T10:00:01Z stderr warning: slow retry\n" +
		"Retention: retained ttl=72h expires=2025-10-25T10:00:00Z cid=bafy-ret-bundle\n"

	if got := buf.String(); got != want {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestLogRecord_PrinterStructuredRendersEnrichedFields runs under `-run TestLogRecord`
// and ensures the CLI prints the canonical stream.LogRecord enriched fields without
// relying on a duplicate payload struct.
func TestLogRecord_PrinterStructuredRendersEnrichedFields(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	p := NewPrinter(FormatStructured, buf)

	p.PrintLog(logstream.LogRecord{
		Timestamp: "2025-10-22T10:00:00Z",
		Stream:    "stdout",
		Line:      "hello",
		NodeID:    "node-abc",
		JobType:   "pre_gate",
		StepIndex: 2000,
		JobID:     "job-123",
	})

	want := "2025-10-22T10:00:00Z stdout node=node-abc job_type=pre_gate step=2000 job=job-123 hello\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintLog() =\n%q\nwant:\n%q", got, want)
	}
}
