package joblist

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	clitui "github.com/iw2rmb/ploy/internal/client/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

var (
	completeGlyphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379"))
	failedGlyphStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75"))
)

// StatusGlyph returns a terminal-status colored glyph for the given job status.
func StatusGlyph(status domaintypes.JobStatus) string {
	switch status {
	case domaintypes.JobStatusSuccess:
		return completeGlyphStyle.Render("⏺")
	case domaintypes.JobStatusFail, domaintypes.JobStatusCancelled:
		return failedGlyphStyle.Render("⏺")
	default:
		return "⣾"
	}
}

// renderPrimaryLine builds the title line for a job row: glyph + name (padded) + duration.
func renderPrimaryLine(job clitui.JobItem) string {
	glyph := StatusGlyph(job.Status)
	name := normalizeLabel(job.Name)
	duration := formatDurationShort(job.DurationMs)
	prefix := glyph + " "
	availableNameWidth := ContentWidth - lipgloss.Width(prefix) - 1 - lipgloss.Width(duration)
	if availableNameWidth < 1 {
		availableNameWidth = 1
	}
	name = truncateRunes(name, availableNameWidth)
	name = name + strings.Repeat(" ", availableNameWidth-lipgloss.Width(name))
	return prefix + name + " " + duration
}

// renderSecondaryLine builds the description line for a job row: job ID.
func renderSecondaryLine(job clitui.JobItem) string {
	return truncateRunes(normalizeLabel(job.JobID.String()), ListWidth)
}

func normalizeLabel(s string) string {
	if t := strings.TrimSpace(s); t != "" {
		return t
	}
	return "-"
}

func formatDurationShort(durationMs int64) string {
	if durationMs <= 0 {
		return "-"
	}
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	}
	if durationMs < 60000 {
		return fmt.Sprintf("%ds", durationMs/1000)
	}
	if durationMs < 3600000 {
		return fmt.Sprintf("%dm", durationMs/60000)
	}
	return fmt.Sprintf("%dh", durationMs/3600000)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}
