package tui

import (
	"slices"

	"charm.land/bubbles/v2/list"

	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenPloyList:
		return m.handleEnterFromRoot()
	case ScreenRunsList:
		return m.handleEnterFromRunsList()
	case ScreenRunDetails:
		if m.rootList.Index() == jobsIndex {
			m.screen = ScreenJobsList
			m.jobList = m.jobList.SetJobs(nil)
			cmd := m.loadJobs()
			return m, cmd
		}
	case ScreenJobsList:
		return m.handleEnterFromJobsList()
	case ScreenUsersList:
		return m.handleEnterFromUsersList()
	}
	return m, nil
}

func (m model) handleEnterFromRoot() (tea.Model, tea.Cmd) {
	switch m.rootList.Index() {
	case runsIndex:
		m.screen = ScreenRunsList
	case jobsIndex:
		m.screen = ScreenJobsList
	case usersIndex:
		if m.usersEnabled {
			m.screen = ScreenUsersList
		}
	}
	return m, nil
}

func (m *model) loadRootPreview() tea.Cmd {
	if m.usersLoading || m.screen != ScreenPloyList || !m.selectedRunID.IsZero() {
		return nil
	}
	switch m.rootList.Index() {
	case runsIndex:
		if !m.runsLoading {
			return m.loadRuns()
		}
	case jobsIndex:
		if !m.jobsLoading {
			return m.loadJobs()
		}
	}
	return nil
}

func (m model) handleEnterFromRunsList() (tea.Model, tea.Cmd) {
	item, ok := m.rightPaneList.SelectedItem().(runItem)
	if !ok {
		return m, nil
	}
	m.setRun(item.run.ID)
	m.rootList.SetItems([]list.Item{item, m.runJobsEntry()})
	m.rootList.Select(runsIndex)
	m.screen = ScreenRunDetails
	return m, loadRunDetailsCmd(m.ctx, m.client, m.baseURL, m.selectedRunID)
}

func (m model) handleEnterFromJobsList() (tea.Model, tea.Cmd) {
	selectedJob, ok := m.jobList.SelectedJob()
	if !ok {
		return m, nil
	}
	m.jobList = m.jobList.SetSelectedJobID(selectedJob.JobID)
	item, _ := m.jobList.ConfirmedItem(m.now, m.spinnerFrame)
	run := m.rootList.Items()[runsIndex]
	if m.selectedRunID.IsZero() {
		run = listItem{title: "Run", description: selectedJob.RunID.String()}
	}
	m.rootList.SetItems([]list.Item{run, item})
	m.rootList.Select(jobsIndex)
	return m, nil
}

func (m *model) setRun(runID domaintypes.RunID) {
	if m.selectedRunID != runID {
		m.jobList = m.jobList.SetJobs(nil)
		m.selectedRunID = runID
		m.runJobTotal = nil
		m.jobsLoading = false
	}
}

func (m *model) loadJobs() tea.Cmd {
	if m.usersLoading {
		return nil
	}
	m.jobsRequest++
	m.jobsLoading = true
	return loadJobsCmd(m.ctx, m.client, m.baseURL, m.selectedRunID, m.jobsRequest, slices.Clone(m.jobList.Jobs()), m.jobsCreatedBy())
}

func (m *model) resetRoot() {
	selected := m.rootList.Index()
	m.rootList.SetItems(buildPloyItems(m.usersEnabled, m.selectedUser, m.usersErr))
	m.rootList.Select(selected)
	m.setRun("")
	m.jobList = m.jobList.SetSelectedJobID("")
}

func (m model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenRunsList, ScreenUsersList:
		m.screen = ScreenPloyList
		m.resetRoot()
	case ScreenRunDetails:
		m.screen = ScreenRunsList
		m.resetRoot()
		m.rootList.Select(runsIndex)
	case ScreenJobsList:
		m.jobList = m.jobList.SetSelectedJobID("")
		if !m.selectedRunID.IsZero() {
			m.screen = ScreenRunDetails
			m.rootList.SetItem(jobsIndex, m.runJobsEntry())
			m.rootList.Select(jobsIndex)
		} else {
			m.screen = ScreenPloyList
			m.resetRoot()
		}
	}
	return m, nil
}
