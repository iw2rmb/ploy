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

// newPloyList creates the left PLOY list with fixed width.
func newPloyList(title string, items []list.Item) list.Model {
	l := newListWithWidth(title, items, ployListWidth)
	l.SetDelegate(newRunDelegate(true, 0))
	return l
}

// The initial width is replaced when the terminal size arrives.
func newRunsList(title string, items []list.Item) list.Model {
	l := newListWithWidth(title, items, runsListWidth)
	l.SetDelegate(newRunDelegate(true, 0))
	return l
}

func (m *model) applyWindowSize() {
	if m.windowWidth > 0 {
		m.rightPaneList.SetWidth(max(1, m.windowWidth-ployListWidth-2))
		m.jobList.SetWidth(max(1, m.windowWidth-ployListWidth-2))
		m.usersList.SetWidth(max(1, m.windowWidth-ployListWidth-2))
	}
	if m.windowHeight <= 0 {
		return
	}
	m.rootList.SetHeight(m.windowHeight)
	m.rightPaneList.SetHeight(m.windowHeight)
	m.jobList.SetHeight(m.windowHeight)
	m.usersList.SetHeight(m.windowHeight)
}

func buildPloyItems(usersEnabled bool, selectedUser string, usersErr error) []list.Item {
	user := "cli-admin only"
	if usersErr != nil {
		user = "unavailable"
	}
	if usersEnabled {
		user = (userItem{username: selectedUser}).Title()
	}
	return []list.Item{
		listItem{title: "Runs", description: "select run"},
		listItem{title: "Jobs", description: "select job"},
		listItem{title: "Users", description: user},
	}
}
