package runs

import (
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestFormatErrorOneLiner(t *testing.T) {
	t.Parallel()

	multiline := "compile\nfailed   at\tstep 2"
	blank := "  \n\t  "

	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{"nil", nil, ""},
		{"blank", &blank, ""},
		{"multiline", &multiline, "compile failed at step 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatErrorOneLiner(tc.input); got != tc.want {
				t.Fatalf("FormatErrorOneLiner(%v)=%q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatNodeID(t *testing.T) {
	t.Parallel()

	var zero domaintypes.NodeID
	node := domaintypes.NodeID(domaintypes.NewNodeKey())

	tests := []struct {
		name  string
		input *domaintypes.NodeID
		want  string
	}{
		{"nil", nil, "-"},
		{"zero", &zero, "-"},
		{"valid", &node, node.String()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatNodeID(tc.input); got != tc.want {
				t.Fatalf("FormatNodeID(%v)=%q, want %q", tc.input, got, tc.want)
			}
		})
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

	tests := []struct {
		name     string
		ms       int64
		started  *time.Time
		finished *time.Time
		now      time.Time
		want     string
	}{
		{"from ms", 2450, nil, nil, now, "2450ms"},
		{"from finished", 0, &started, &finished, time.Time{}, "2.5s"},
		{"from running elapsed", 0, &started, nil, now, "1.5s"},
		{"empty", 0, nil, nil, now, "-"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatDurationMsOrElapsed(tc.ms, tc.started, tc.finished, tc.now); got != tc.want {
				t.Fatalf("FormatDurationMsOrElapsed=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatDurationForStatus(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	finished := started.Add(2500 * time.Millisecond)
	now := started.Add(1500 * time.Millisecond)

	tests := []struct {
		name     string
		status   string
		ms       int64
		started  *time.Time
		finished *time.Time
		now      time.Time
		want     string
	}{
		{"failed uses compact", "failed", 2450, nil, nil, now, "2.5s"},
		{"success from finished", "success", 0, &started, &finished, now, "2.5s"},
		{"running uses ms", "running", 2450, nil, nil, now, "2450ms"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatDurationForStatus(tc.status, tc.ms, tc.started, tc.finished, tc.now); got != tc.want {
				t.Fatalf("FormatDurationForStatus=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusGlyph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		frame  int
		want   string
	}{
		{"running", 0, "⣾"},
		{"running", 1, "⣷"},
		{"started", 0, "⣾"},
		{"failed", 0, "✗"},
		{"queued", 0, "·"},
		{"finished", 0, "✓"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := StatusGlyph(tc.status, tc.frame); got != tc.want {
				t.Fatalf("StatusGlyph(%q,%d)=%q, want %q", tc.status, tc.frame, got, tc.want)
			}
		})
	}
}

func TestColoredStatusGlyph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   string
	}{
		{"failed", "\x1b[91m✗\x1b[0m"},
		{"running", "\x1b[92m⣾\x1b[0m"},
		{"success", "\x1b[92m✓\x1b[0m"},
		{"queued", "\x1b[39m·\x1b[0m"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := ColoredStatusGlyph(tc.status, 0); got != tc.want {
				t.Fatalf("ColoredStatusGlyph(%q,0)=%q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
