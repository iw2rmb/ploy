package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

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

// newJobsList creates a jobs list with a 48-column width.
func newJobsList(title string, items []list.Item) list.Model {
	return newListWithWidth(title, items, jobsListWidth)
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

func buildRunDetailsPloyItems(migName string, migID domaintypes.MigID, runID domaintypes.RunID, jobsTotal string) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	runDesc := strings.TrimSpace(runID.String())
	if runDesc == "" {
		runDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Run", description: runDesc},
		listItem{title: "Jobs", description: jobsTotal},
	}
}

func buildMigrationDetailsPloyItems(migName string, migID domaintypes.MigID, runsTotal string) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Runs", description: runsTotal},
		listItem{title: "Jobs", description: "select job"},
	}
}

func buildJobDetailsPloyItems(migName string, migID domaintypes.MigID, runID domaintypes.RunID, jobID domaintypes.JobID) []list.Item {
	migTitle := strings.TrimSpace(migName)
	if migTitle == "" {
		migTitle = "-"
	}
	migDesc := strings.TrimSpace(migID.String())
	if migDesc == "" {
		migDesc = "-"
	}
	runDesc := strings.TrimSpace(runID.String())
	if runDesc == "" {
		runDesc = "-"
	}
	jobDesc := strings.TrimSpace(jobID.String())
	if jobDesc == "" {
		jobDesc = "-"
	}
	return []list.Item{
		listItem{title: migTitle, description: migDesc},
		listItem{title: "Run", description: runDesc},
		listItem{title: "Job", description: jobDesc},
	}
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
