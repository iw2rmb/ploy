package runs

import (
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestFormatErrorOneLiner(t *testing.T) {
	t.Parallel()

	if got := FormatErrorOneLiner(nil); got != "" {
		t.Fatalf("FormatErrorOneLiner(nil)=%q, want empty", got)
	}

	blank := "  \n\t  "
	if got := FormatErrorOneLiner(&blank); got != "" {
		t.Fatalf("FormatErrorOneLiner(blank)=%q, want empty", got)
	}

	raw := "compile\nfailed   at\tstep 2"
	if got, want := FormatErrorOneLiner(&raw), "compile failed at step 2"; got != want {
		t.Fatalf("FormatErrorOneLiner(multiline)=%q, want %q", got, want)
	}
}

func TestFormatNodeID(t *testing.T) {
	t.Parallel()

	if got := FormatNodeID(nil); got != "-" {
		t.Fatalf("FormatNodeID(nil)=%q, want -", got)
	}

	var zero domaintypes.NodeID
	if got := FormatNodeID(&zero); got != "-" {
		t.Fatalf("FormatNodeID(zero)=%q, want -", got)
	}

	node := domaintypes.NodeID(domaintypes.NewNodeKey())
	if got := FormatNodeID(&node); got != node.String() {
		t.Fatalf("FormatNodeID(valid)=%q, want %q", got, node.String())
	}
}

func TestFormatDurationCompact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ms   int64
		want string
	}{
		{ms: 0, want: "-"},
		{ms: 12, want: "12ms"},
		{ms: 2450, want: "2.5s"},
	}

	for _, tc := range tests {
		if got := FormatDurationCompact(tc.ms); got != tc.want {
			t.Fatalf("FormatDurationCompact(%d)=%q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestFormatDurationMsOrElapsed(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	finished := started.Add(2500 * time.Millisecond)
	now := started.Add(1500 * time.Millisecond)

	if got, want := FormatDurationMsOrElapsed(2450, nil, nil, now), "2450ms"; got != want {
		t.Fatalf("FormatDurationMsOrElapsed(ms)=%q, want %q", got, want)
	}
	if got, want := FormatDurationMsOrElapsed(0, &started, &finished, time.Time{}), "2.5s"; got != want {
		t.Fatalf("FormatDurationMsOrElapsed(finished)=%q, want %q", got, want)
	}
	if got, want := FormatDurationMsOrElapsed(0, &started, nil, now), "1.5s"; got != want {
		t.Fatalf("FormatDurationMsOrElapsed(running)=%q, want %q", got, want)
	}
	if got, want := FormatDurationMsOrElapsed(0, nil, nil, now), "-"; got != want {
		t.Fatalf("FormatDurationMsOrElapsed(empty)=%q, want %q", got, want)
	}
}

func TestFormatDurationForStatus(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	finished := started.Add(2500 * time.Millisecond)
	now := started.Add(1500 * time.Millisecond)

	if got, want := FormatDurationForStatus("failed", 2450, nil, nil, now), "2.5s"; got != want {
		t.Fatalf("FormatDurationForStatus(failed)=%q, want %q", got, want)
	}
	if got, want := FormatDurationForStatus("success", 0, &started, &finished, now), "2.5s"; got != want {
		t.Fatalf("FormatDurationForStatus(success finished)=%q, want %q", got, want)
	}
	if got, want := FormatDurationForStatus("running", 2450, nil, nil, now), "2450ms"; got != want {
		t.Fatalf("FormatDurationForStatus(running)=%q, want %q", got, want)
	}
}

func TestStatusGlyph(t *testing.T) {
	t.Parallel()

	if got := StatusGlyph("running", 0); got != "⣾" {
		t.Fatalf("StatusGlyph(running,0)=%q, want %q", got, "⣾")
	}
	if got := StatusGlyph("running", 1); got != "⣷" {
		t.Fatalf("StatusGlyph(running,1)=%q, want %q", got, "⣷")
	}
	if got := StatusGlyph("started", 0); got != "⣾" {
		t.Fatalf("StatusGlyph(started,0)=%q, want %q", got, "⣾")
	}
	if got := StatusGlyph("failed", 0); got != "✗" {
		t.Fatalf("StatusGlyph(failed,0)=%q, want %q", got, "✗")
	}
	if got := StatusGlyph("queued", 0); got != "·" {
		t.Fatalf("StatusGlyph(queued,0)=%q, want %q", got, "·")
	}
}

func TestColoredStatusGlyph(t *testing.T) {
	t.Parallel()

	if got := ColoredStatusGlyph("failed", 0); got != "\x1b[91m✗\x1b[0m" {
		t.Fatalf("ColoredStatusGlyph(failed,0)=%q, want light-red fail glyph", got)
	}
	if got := ColoredStatusGlyph("running", 0); got != "⣾" {
		t.Fatalf("ColoredStatusGlyph(running,0)=%q, want spinner", got)
	}
}
