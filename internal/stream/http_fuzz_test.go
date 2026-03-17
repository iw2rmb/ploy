package logstream

import (
	"bytes"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// FuzzWriteEventFrame ensures WriteEventFrame handles arbitrary data payloads
// (including newlines and non-UTF8) without panicking and produces a frame
// that ends with a blank line per SSE framing rules.
func FuzzWriteEventFrame(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte("line1\nline2\nline3"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var buf bytes.Buffer
		evt := Event{ID: 1, Type: domaintypes.SSEEventLog, Data: data}
		if err := WriteEventFrame(&buf, evt); err != nil {
			t.Fatalf("WriteEventFrame error: %v", err)
		}
		out := buf.String()
		if len(out) == 0 {
			t.Fatal("no output produced")
		}
		// Every frame must end with a blank line.
		if !bytes.HasSuffix([]byte(out), []byte("\n\n")) {
			t.Fatalf("frame does not end with blank line: %q", out)
		}
		// Must include event type marker.
		if !bytes.Contains([]byte(out), []byte("event: log\n")) {
			t.Fatalf("missing event type line: %q", out)
		}
		// Must include id marker when ID > 0.
		if !bytes.Contains([]byte(out), []byte("id: 1\n")) {
			t.Fatalf("missing id line: %q", out)
		}
		// Should contain at least one data line (empty is allowed but present).
		if !bytes.Contains([]byte(out), []byte("data:")) {
			t.Fatalf("missing data line: %q", out)
		}
	})
}
