package tui

import (
	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleEnter implements Enter-key transitions per the state machine contract.
func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenRoot:
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
	switch m.ploy.Index() {
	case 0: // Migrations
		m.screen = ScreenMigrationsList
		m.secondary = newList("MIGRATIONS", nil)
		m.applyWindowHeight()
		return m, loadMigsCmd(m.client, m.baseURL)
	case 1: // Runs
		m.screen = ScreenRunsList
		m.secondary = newRunsList("RUNS", nil)
		m.applyWindowHeight()
		return m, loadRunsCmd(m.client, m.baseURL)
	case 2: // Jobs
		m.screen = ScreenJobsList
		m.secondary = newJobsList("JOBS", nil)
		m.applyWindowHeight()
		return m, tea.Batch(loadJobsCmd(m.client, m.baseURL, nil), loadRunsCmd(m.client, m.baseURL))
	}
	return m, nil
}

func (m model) handleEnterFromMigrationsList() (tea.Model, tea.Cmd) {
	item, ok := m.secondary.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	m.selectedMigID = domaintypes.MigID(item.description)
	m.selectedMigName = item.title
	m.selectedRunID = ""
	m.selectedJobID = ""
	m.ploy.SetItems(buildMigrationDetailsPloyItems(
		m.selectedMigName,
		m.selectedMigID,
		"total: —",
	))
	m.ploy.Select(0)
	m.applyWindowHeight()
	m.screen = ScreenMigrationDetails
	return m, loadMigDetailsCmd(m.client, m.baseURL, m.selectedMigID)
}

func (m model) handleEnterFromRunsList() (tea.Model, tea.Cmd) {
	item, ok := m.secondary.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	m.selectedRunID = domaintypes.RunID(item.title)
	m.selectedJobID = ""
	m.selectedMigID = ""
	m.selectedMigName = ""
	for _, run := range m.runs {
		if run.ID == m.selectedRunID {
			m.selectedMigID = run.MigID
			m.selectedMigName = run.MigName
			break
		}
	}
	m.ploy.SetItems(buildRunDetailsPloyItems(
		m.selectedMigName,
		m.selectedMigID,
		m.selectedRunID,
		"total: —",
	))
	m.ploy.Select(1)
	m.applyWindowHeight()
	m.screen = ScreenRunDetails
	return m, loadRunDetailsCmd(m.client, m.baseURL, m.selectedRunID)
}

func (m model) handleEnterFromJobsList() (tea.Model, tea.Cmd) {
	item, ok := m.secondary.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	m.selectedJobID = domaintypes.JobID(item.description)
	selectedJob, foundJob := findJobByID(m.jobs, m.selectedJobID)
	if !foundJob {
		return m, nil
	}
	m.selectedRunID = selectedJob.RunID
	m.selectedMigName = selectedJob.MigName
	m.selectedMigID = ""
	for _, run := range m.runs {
		if run.ID == m.selectedRunID {
			m.selectedMigID = run.MigID
			break
		}
	}
	m.ploy.SetItems(buildJobDetailsPloyItems(
		m.selectedMigName,
		m.selectedMigID,
		m.selectedRunID,
		m.selectedJobID,
	))
	m.ploy.Select(2)
	return m, nil
}

func findJobByID(jobs []clitui.JobItem, jobID domaintypes.JobID) (clitui.JobItem, bool) {
	for _, job := range jobs {
		if job.JobID == jobID {
			return job, true
		}
	}
	return clitui.JobItem{}, false
}

// handleEsc implements Esc-key transitions per the state machine contract.
func (m model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenMigrationsList:
		m.screen = ScreenRoot
	case ScreenMigrationDetails:
		m.screen = ScreenMigrationsList
		m.setPloySelectionState(true, false, false)
	case ScreenRunsList:
		m.screen = ScreenRoot
	case ScreenRunDetails:
		m.screen = ScreenRunsList
		m.setPloySelectionState(true, true, false)
	case ScreenJobsList:
		m.screen = ScreenRoot
	case ScreenRoot:
		return m, tea.Quit
	}
	return m, nil
}
