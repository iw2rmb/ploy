package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	var content string
	switch m.screen {
	case S1Root:
		content = m.ploy.View()
	case S2MigrationsList:
		content = strings.Join([]string{m.ploy.View(), m.secondary.View()}, "  ")
	case S3MigrationDetails:
		content = m.detail.View()
	case S4RunsList:
		content = strings.Join([]string{m.ploy.View(), m.secondary.View()}, "  ")
	case S5RunDetails:
		content = m.detail.View()
	case S6JobsList:
		content = strings.Join([]string{m.ploy.View(), m.secondary.View()}, "  ")
	}
	return tea.NewView(content)
}
