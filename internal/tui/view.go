package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	var content string
	if m.screen == ScreenRunsList || m.screen == ScreenJobsList || m.screen == ScreenUsersList {
		delegate := list.NewDefaultDelegate()
		delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.BorderStyle(lipgloss.HiddenBorder())
		delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.BorderStyle(lipgloss.HiddenBorder())
		m.rootList.SetDelegate(delegate)
	}
	content = m.rootList.View()
	var right string
	switch {
	case m.runsVisible():
		m.rightPaneList.SetDelegate(newRunDelegate(m.screen == ScreenRunsList, m.spinnerFrame))
		right = m.rightPaneList.View()
	case m.jobsVisible():
		right = m.jobList.View(m.screen == ScreenJobsList, m.now, m.spinnerFrame)
	case m.usersVisible():
		delegate := list.NewDefaultDelegate()
		if m.screen != ScreenUsersList {
			delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.BorderStyle(lipgloss.HiddenBorder())
			delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.BorderStyle(lipgloss.HiddenBorder())
		}
		m.usersList.SetDelegate(delegate)
		right = m.usersList.View()
	}
	if right != "" {
		ploy := lipgloss.NewStyle().Width(ployListWidth).Render(content)
		content = lipgloss.JoinHorizontal(lipgloss.Top, ploy, "  ", right)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}
