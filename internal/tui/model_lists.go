package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"
)

// normalizeLabel returns the trimmed string, or "-" if it's blank.
func normalizeLabel(s string) string {
	if t := strings.TrimSpace(s); t != "" {
		return t
	}
	return "-"
}

// newListWithWidth creates a list with the shared TUI invariants applied:
// - help disabled
// - quit keybindings disabled
func newListWithWidth(title string, items []list.Item, width int) list.Model {
	l := list.New(items, list.NewDefaultDelegate(), width, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	return l
}

// newList creates a standard-width list (24 columns).
func newList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, listWidth)
}

// newPloyList creates the left PLOY list with fixed width.
func newPloyList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, ployListWidth)
}

// newRunsList creates a runs list with a 30-column width.
func newRunsList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, runsListWidth)
}

// applyWindowHeight reapplies the cached terminal height to all lists.
func (m *model) applyWindowHeight() {
	if m.windowHeight <= 0 {
		return
	}
	m.ploy.SetHeight(m.windowHeight)
	m.secondary.SetHeight(m.windowHeight)
	m.detail.SetHeight(m.windowHeight)
	m.jobList.SetHeight(m.windowHeight)
}

// buildPloyItems constructs the left PLOY list with context-aware labels.
func buildPloyItems(hasMigration, hasRun, hasJob bool) []list.Item {
	migrationsTitle := "Migrations"
	if hasMigration {
		migrationsTitle = "Migration"
	}
	runsTitle := "Runs"
	if hasRun {
		runsTitle = "Run"
	}
	jobsTitle := "Jobs"
	if hasJob {
		jobsTitle = "Job"
	}
	return []list.Item{
		listItem{title: migrationsTitle, description: "select migration"},
		listItem{title: runsTitle, description: "select run"},
		listItem{title: jobsTitle, description: "select job"},
	}
}

// ployEntry holds a title-description pair for the PLOY detail list.
type ployEntry struct{ title, desc string }

// buildDetailsPloyItems constructs a PLOY detail list, normalizing each entry.
func buildDetailsPloyItems(entries []ployEntry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = listItem{title: normalizeLabel(e.title), description: normalizeLabel(e.desc)}
	}
	return items
}

// setPloySelectionState applies root item label state while preserving cursor.
func (m *model) setPloySelectionState(hasMigration, hasRun, hasJob bool) {
	m.hasSelectedMigration = hasMigration
	m.hasSelectedRun = hasRun
	m.hasSelectedJob = hasJob

	selectedIdx := m.ploy.Index()
	m.ploy.SetItems(buildPloyItems(hasMigration, hasRun, hasJob))
	if selectedIdx < 0 {
		selectedIdx = 0
	}
	if itemCount := len(m.ploy.Items()); itemCount > 0 {
		if selectedIdx >= itemCount {
			selectedIdx = itemCount - 1
		}
		m.ploy.Select(selectedIdx)
	}
}

// setWindowHeight updates cached terminal height and applies it to all lists.
func (m *model) setWindowHeight(height int) {
	if height <= 0 {
		return
	}
	m.windowHeight = height
	m.applyWindowHeight()
}
