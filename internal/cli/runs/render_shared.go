package runs

import (
	"fmt"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// SpinnerFrames defines deterministic spinner glyph order for running states.
var SpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// FormatErrorOneLiner normalizes a multi-line error to a single readable line.
func FormatErrorOneLiner(lastErr *string) string {
	if lastErr == nil {
		return ""
	}
	fields := strings.Fields(*lastErr)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// FormatNodeID renders node ID values with the shared placeholder semantics.
func FormatNodeID(nodeID *domaintypes.NodeID) string {
	if nodeID == nil || nodeID.IsZero() {
		return "-"
	}
	return nodeID.String()
}

// FormatDurationCompact renders duration_ms in status-report compact form.
func FormatDurationCompact(durationMs int64) string {
	if durationMs <= 0 {
		return "-"
	}
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	}
	return fmt.Sprintf("%.1fs", float64(durationMs)/1000.0)
}

// FormatDurationMsOrElapsed renders duration for follow-mode rows.
// It prefers duration_ms; otherwise falls back to timestamps.
func FormatDurationMsOrElapsed(durationMs int64, startedAt, finishedAt *time.Time, now time.Time) string {
	if durationMs > 0 {
		return fmt.Sprintf("%dms", durationMs)
	}
	if finishedAt != nil && startedAt != nil {
		d := finishedAt.Sub(*startedAt)
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if startedAt != nil {
		if now.IsZero() {
			now = time.Now()
		}
		d := now.Sub(*startedAt)
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return "-"
}

// StatusGlyph maps a status to a deterministic display glyph.
func StatusGlyph(status string, spinnerFrame int) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "started":
		return spinnerAtFrame(spinnerFrame)
	case "success", "succeeded":
		return "✓"
	case "fail", "failed", "crash", "crashed", "error":
		return "✗"
	case "cancelled", "canceled":
		return "○"
	case "created", "queued":
		return "·"
	default:
		return " "
	}
}

func spinnerAtFrame(frame int) string {
	if len(SpinnerFrames) == 0 {
		return " "
	}
	index := ((-frame)%len(SpinnerFrames) + len(SpinnerFrames)) % len(SpinnerFrames)
	return SpinnerFrames[index]
}
