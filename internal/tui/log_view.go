package tui

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iw2rmb/ploy/internal/stream"
)

var stderrTimeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

func (m model) renderJobLog(width int) string {
	styles := m.rootList.Styles
	title := styles.TitleBar.Render(styles.Title.UnsetBackground().UnsetForeground().Render("LOG"))
	available := m.windowHeight - lipgloss.Height(title)
	var rows []string
	if m.jobLog.err != nil {
		rows = strings.Split(ansi.Hardwrap(plainLogText(m.jobLog.err.Error()), width, true), "\n")
		slices.Reverse(rows)
	} else if m.jobLog.done && len(m.jobLog.lines) == 0 {
		rows = []string{"No output"}
	}

	// Wrap from the tail so rendering cost depends on the visible records.
	for i := len(m.jobLog.lines) - 1; i >= 0 && len(rows) < available; i-- {
		lines := renderLogRecord(m.jobLog.lines[i], width)
		slices.Reverse(lines)
		rows = append(rows, lines...)
	}
	rows = rows[:min(len(rows), max(0, available))]
	slices.Reverse(rows)
	if len(rows) == 0 {
		return title
	}
	return title + "\n" + strings.Join(rows, "\n")
}

func plainLogText(text string) string {
	text = strings.ReplaceAll(ansi.Strip(text), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func renderLogRecord(record logstream.LogRecord, width int) []string {
	timestamp := "--:--:--"
	if t, err := time.Parse(time.RFC3339Nano, record.Timestamp); err == nil {
		timestamp = t.UTC().Format(time.TimeOnly)
	}
	if record.Stream == "stderr" {
		timestamp = stderrTimeStyle.Render(timestamp)
	}
	const indent = "         "
	text := plainLogText(record.Line)
	if width <= len(indent) {
		return append([]string{ansi.Truncate(timestamp, width, "")}, strings.Split(ansi.Hardwrap(text, width, true), "\n")...)
	}
	rows := strings.Split(ansi.Hardwrap(text, width-len(indent), true), "\n")
	rows[0] = timestamp + " " + rows[0]
	for i := 1; i < len(rows); i++ {
		rows[i] = indent + rows[i]
	}
	return rows
}
