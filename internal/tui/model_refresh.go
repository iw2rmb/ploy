package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type refreshTickMsg time.Time

func (m model) runsVisible() bool {
	return m.screen == ScreenRunsList || (m.screen == ScreenPloyList && m.selectedRunID.IsZero() && m.rootList.Index() == runsIndex)
}

func (m model) jobsVisible() bool {
	return m.screen == ScreenJobsList || (m.screen == ScreenPloyList && m.selectedRunID.IsZero() && m.rootList.Index() == jobsIndex)
}

func (m model) activeRunsVisible() bool {
	if !m.runsVisible() {
		return false
	}
	for _, item := range m.rightPaneList.Items() {
		run := item.(runItem).run
		if run.Status == domaintypes.RunStatusRunning || run.Status == domaintypes.RunStatusQueued {
			return true
		}
	}
	return false
}

func (m model) activeJobsVisible() bool {
	return m.jobsVisible() && m.jobList.HasActiveJobs()
}

func (m *model) scheduleRefresh() tea.Cmd {
	if m.refreshScheduled || (!m.activeRunsVisible() && !m.activeJobsVisible()) {
		return nil
	}
	m.refreshScheduled = true
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return refreshTickMsg(now) })
}

func (m model) handleRefreshTick(now time.Time) (tea.Model, tea.Cmd) {
	m.refreshScheduled = false
	m.now = now
	m.spinnerFrame = (m.spinnerFrame + 1) % len(cliruns.SpinnerFrames)
	if item, ok := m.jobList.ConfirmedItem(m.now, m.spinnerFrame); ok {
		m.rootList.SetItem(jobsIndex, item)
	}
	var commands []tea.Cmd
	if m.activeRunsVisible() && !m.runsLoading {
		commands = append(commands, m.loadRuns())
	}
	if m.activeJobsVisible() && !m.jobsLoading {
		commands = append(commands, m.loadJobs())
	}
	return m, tea.Batch(commands...)
}

func (m *model) loadRuns() tea.Cmd {
	if m.usersLoading {
		return nil
	}
	m.runsRequest++
	m.runsLoading = true
	createdBy, creator, all := m.runsCreatorScope()
	return loadRunsCmd(m.ctx, m.client, m.baseURL, m.runsRequest, createdBy, creator, all)
}
