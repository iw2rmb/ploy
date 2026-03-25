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
	case ScreenMigrationsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.secondary.View())
	case ScreenMigrationDetails:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.detail.View())
	case ScreenRunsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.secondary.View())
	case ScreenRunDetails:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.detail.View())
	case ScreenJobsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.ploy.View(), "  ", m.secondary.View())
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}
