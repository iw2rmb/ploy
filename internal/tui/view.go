package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	var content string
	switch m.screen {
	case ScreenPloyList:
		if m.rootList.Index() == 2 {
			content = lipgloss.JoinHorizontal(lipgloss.Top, m.rootList.View(), "  ", m.jobList.View())
		} else {
			content = m.rootList.View()
		}
	case ScreenMigrationsList, ScreenRunsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.rootList.View(), "  ", m.rightPaneList.View())
	case ScreenJobsList:
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.rootList.View(), "  ", m.jobList.View())
	case ScreenMigrationDetails, ScreenRunDetails:
		content = m.rootList.View()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}
