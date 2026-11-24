package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	rootFocused := m.screen == ScreenPloyList || m.screen == ScreenRunDetails
	m.rootList.SetDelegate(newRunDelegate(rootFocused, m.spinnerFrame))
	content := renderList(m.rootList, rootFocused)
	var right string
	switch {
	case m.runsVisible():
		focused := m.screen == ScreenRunsList
		m.rightPaneList.SetDelegate(newRunDelegate(focused, m.spinnerFrame))
		right = renderList(m.rightPaneList, focused)
	case m.jobsVisible():
		right = m.jobList.View(m.screen == ScreenJobsList, m.now, m.spinnerFrame)
	case m.usersVisible():
		delegate := list.NewDefaultDelegate()
		focused := m.screen == ScreenUsersList
		if !focused {
			delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.BorderStyle(lipgloss.HiddenBorder())
			delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.BorderStyle(lipgloss.HiddenBorder())
		}
		m.usersList.SetDelegate(delegate)
		right = renderList(m.usersList, focused)
	}
	if right != "" {
		ploy := lipgloss.NewStyle().Width(ployListWidth).Render(content)
		content = lipgloss.JoinHorizontal(lipgloss.Top, ploy, "  ", right)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func renderList(l list.Model, focused bool) string {
	if !focused {
		l.Styles.Title = l.Styles.Title.UnsetBackground().UnsetForeground()
	}
	return l.View()
}
