package tui

import (
	tea "charm.land/bubbletea/v2"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleEnter implements Enter-key transitions per the state machine contract.
func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenPloyList:
		return m.handleEnterFromRoot()
	case ScreenMigrationsList:
		return m.handleEnterFromMigrationsList()
	case ScreenRunsList:
		return m.handleEnterFromRunsList()
	case ScreenJobsList:
		return m.handleEnterFromJobsList()
	}
	return m, nil
}

func (m model) handleEnterFromRoot() (tea.Model, tea.Cmd) {
	switch m.rootList.Index() {
	case 0: // Migrations
		m.screen = ScreenMigrationsList
		m.rightPaneList = newList("MIGRATIONS", nil)
		m.applyWindowHeight()
		return m, loadMigsCmd(m.client, m.baseURL)
	case 1: // Runs
		m.screen = ScreenRunsList
		m.rightPaneList = newRunsList("RUNS", nil)
		m.applyWindowHeight()
		return m, loadRunsCmd(m.client, m.baseURL)
	case 2: // Jobs
		m.screen = ScreenJobsList
		m.applyWindowHeight()
		return m, tea.Batch(loadJobsCmd(m.client, m.baseURL, nil), loadRunsCmd(m.client, m.baseURL))
	}
	return m, nil
}

func (m model) handleEnterFromMigrationsList() (tea.Model, tea.Cmd) {
	item, ok := m.rightPaneList.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	m.selectedMigID = domaintypes.MigID(item.description)
	m.selectedMigName = item.title
	m.selectedRunID = ""
	m.jobList = m.jobList.SetSelectedJobID("")
	m.rootList.SetItems(buildDetailsPloyItems([]ployEntry{
		{title: m.selectedMigName, desc: m.selectedMigID.String()},
		{title: "Runs", desc: "total: —"},
		{title: "Jobs", desc: "select job"},
	}))
	m.rootList.Select(0)
	m.applyWindowHeight()
	m.screen = ScreenMigrationDetails
	return m, loadMigDetailsCmd(m.client, m.baseURL, m.selectedMigID)
}

func (m model) handleEnterFromRunsList() (tea.Model, tea.Cmd) {
	item, ok := m.rightPaneList.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	m.selectedRunID = domaintypes.RunID(item.title)
	m.jobList = m.jobList.SetSelectedJobID("")
	m.selectedMigID = ""
	m.selectedMigName = ""
	m.resolveMigContext(m.selectedRunID)
	m.rootList.SetItems(buildDetailsPloyItems([]ployEntry{
		{title: m.selectedMigName, desc: m.selectedMigID.String()},
		{title: "Run", desc: m.selectedRunID.String()},
		{title: "Jobs", desc: "total: —"},
	}))
	m.rootList.Select(1)
	m.applyWindowHeight()
	m.screen = ScreenRunDetails
	return m, loadRunDetailsCmd(m.client, m.baseURL, m.selectedRunID)
}

func (m model) handleEnterFromJobsList() (tea.Model, tea.Cmd) {
	selectedJob, ok := m.jobList.SelectedJob()
	if !ok {
		return m, nil
	}
	m.jobList = m.jobList.SetSelectedJobID(selectedJob.JobID)
	m.selectedRunID = selectedJob.RunID
	m.selectedMigName = selectedJob.MigName
	m.selectedMigID = ""
	m.resolveMigContext(m.selectedRunID)
	m.rootList.SetItems(buildDetailsPloyItems([]ployEntry{
		{title: m.selectedMigName, desc: m.selectedMigID.String()},
		{title: "Run", desc: m.selectedRunID.String()},
		{title: "Job", desc: m.jobList.ConfirmedJobID().String()},
	}))
	m.rootList.Select(2)
	return m, loadJobDetailsCmd(m.client, m.baseURL, selectedJob.RunID, selectedJob.RepoID, selectedJob.JobID)
}

// resolveMigContext populates selectedMigID and selectedMigName from the runs cache.
func (m *model) resolveMigContext(runID domaintypes.RunID) {
	for _, run := range m.runs {
		if run.ID == runID {
			m.selectedMigID = run.MigID
			m.selectedMigName = run.MigName
			return
		}
	}
}

// handleEsc implements Esc-key transitions per the state machine contract.
func (m model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenMigrationsList:
		m.screen = ScreenPloyList
	case ScreenMigrationDetails:
		m.screen = ScreenMigrationsList
		m.setPloySelectionState(true, false, false)
	case ScreenRunsList:
		m.screen = ScreenPloyList
	case ScreenRunDetails:
		m.screen = ScreenRunsList
		m.setPloySelectionState(true, true, false)
	case ScreenJobsList:
		m.screen = ScreenPloyList
	case ScreenPloyList:
		return m, tea.Quit
	}
	return m, nil
}
