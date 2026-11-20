package tui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type userItem struct{ username string }

func (i userItem) Title() string {
	if i.username == "" {
		return "All"
	}
	return "User: " + i.username
}

func (i userItem) Description() string { return "" }
func (i userItem) FilterValue() string { return i.Title() }

func (m model) handleUsersLoaded(msg usersLoadedMsg) (tea.Model, tea.Cmd) {
	m.usersLoading = false
	m.usersEnabled = msg.enabled && msg.err == nil
	m.usersErr = msg.err
	if m.usersEnabled {
		items := []list.Item{userItem{}}
		for _, username := range msg.users {
			items = append(items, userItem{username: username})
		}
		m.usersList.SetItems(items)
	}
	m.resetRoot()
	m.applyWindowSize()
	createdBy, creator, all := m.runsCreatorScope()
	commands := []tea.Cmd{loadRunsCmd(m.ctx, m.client, m.baseURL, m.runsRequest, createdBy, creator, all)}
	if m.jobsVisible() {
		commands = append(commands, m.loadJobs())
	}
	return m, tea.Batch(commands...)
}

func (m model) runsCreatorScope() (string, string, bool) {
	if m.usersEnabled {
		return "", m.selectedUser, m.selectedUser == ""
	}
	return strings.TrimSpace(os.Getenv("USER")), "", false
}

func (m model) jobsCreatedBy() string {
	if m.usersEnabled {
		return m.selectedUser
	}
	return ""
}

func (m model) usersVisible() bool {
	return m.usersEnabled && (m.screen == ScreenUsersList || (m.screen == ScreenPloyList && m.rootList.Index() == usersIndex))
}

func (m model) handleEnterFromUsersList() (tea.Model, tea.Cmd) {
	selected, ok := m.usersList.SelectedItem().(userItem)
	if !ok {
		return m, nil
	}
	m.screen = ScreenPloyList
	if m.selectedUser == selected.username {
		return m, nil
	}
	m.selectedUser = selected.username
	m.runsLoading = false
	m.jobsLoading = false
	m.rightPaneList = newRunsList("RUNS", nil)
	m.jobList = m.jobList.SetJobs(nil)
	m.selectedRunID = ""
	m.runJobTotal = nil
	m.resetRoot()
	m.rootList.Select(usersIndex)
	m.applyWindowSize()
	cmd := tea.Batch(m.loadRuns(), m.loadJobs())
	return m, cmd
}
