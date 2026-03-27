package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func renderJobsPrimaryLine(job clitui.JobItem) string {
	glyph := jobsStatusGlyph(job.Status)
	name := normalizeLabel(job.Name)
	duration := formatDurationShort(job.DurationMs)
	prefix := glyph + " "
	availableNameWidth := jobsContentWidth - lipgloss.Width(prefix) - 1 - lipgloss.Width(duration)
	if availableNameWidth < 1 {
		availableNameWidth = 1
	}
	name = truncateRunes(name, availableNameWidth)
	name = name + strings.Repeat(" ", availableNameWidth-lipgloss.Width(name))
	return prefix + name + " " + duration
}

func renderJobsSecondaryLine(job clitui.JobItem) string {
	return truncateRunes(normalizeLabel(job.JobID.String()), jobsListWidth)
}

func jobsStatusGlyph(status domaintypes.JobStatus) string {
	switch status {
	case domaintypes.JobStatusSuccess:
		return jobsCompleteGlyphStyle.Render("⏺")
	case domaintypes.JobStatusFail, domaintypes.JobStatusCancelled:
		return jobsFailedGlyphStyle.Render("⏺")
	default:
		return "⣾"
	}
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
