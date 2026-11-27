package tui

import (
	"os"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

// View satisfies tea.Model and renders the current screen.
func (m model) View() tea.View {
	rootFocused := m.screen == ScreenPloyList || m.screen == ScreenRunDetails
	m.rootList.SetDelegate(newRunDelegate(rootFocused, m.spinnerFrame))
	content := renderList(m.rootList, rootFocused)
	run, showRun := m.visibleRun()
	remaining := m.rightPaneList.Width()
	var right string
	switch {
	case m.runsVisible():
		if showRun {
			width := min(runsListWidth, max(1, (remaining-2)/2))
			m.rightPaneList.SetWidth(width)
			remaining -= width + 2
		}
		focused := m.screen == ScreenRunsList
		m.rightPaneList.SetDelegate(newRunDelegate(focused, m.spinnerFrame))
		right = renderList(m.rightPaneList, focused)
		if showRun {
			right = lipgloss.NewStyle().Width(m.rightPaneList.Width()).MaxWidth(m.rightPaneList.Width()).Render(right)
		}
	case m.jobsVisible():
		showLog := !m.highlightedLogJob().IsZero()
		if showLog {
			width := min(joblist.ListWidth, max(1, (remaining-2)/2))
			m.jobList.SetWidth(width)
			remaining -= width + 2
		}
		right = m.jobList.View(m.screen == ScreenJobsList, m.now, m.spinnerFrame)
		if showLog && remaining > 0 {
			right = lipgloss.NewStyle().Width(m.jobList.Width()).MaxWidth(m.jobList.Width()).Render(right)
			right = lipgloss.JoinHorizontal(lipgloss.Top, right, "  ", m.renderJobLog(remaining))
		}
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
	if showRun && remaining > 0 {
		m.runDetailsList.SetSize(remaining, m.windowHeight)
		m.runDetailsList.SetItems(runDetailItems(run.run, m.runDetails.nodeName, common.SupportsOSC8(os.Stdout)))
		details := renderList(m.runDetailsList, false)
		if right == "" {
			right = details
		} else {
			right = lipgloss.JoinHorizontal(lipgloss.Top, right, "  ", details)
		}
	}
	if right != "" {
		ploy := lipgloss.NewStyle().Width(ployListWidth).Render(content)
		content = lipgloss.JoinHorizontal(lipgloss.Top, ploy, "  ", right)
	}
	content = lipgloss.NewStyle().MaxWidth(m.windowWidth).MaxHeight(m.windowHeight).Render(content)
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
