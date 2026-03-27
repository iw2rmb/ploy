package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	var content string
	switch m.screen {
	case ScreenRoot:
		content = m.ploy.View()
	case ScreenMigrationsList, ScreenRunsList, ScreenJobsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.secondary.View())
	case ScreenMigrationDetails, ScreenRunDetails:
		content = m.ploy.View()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}
