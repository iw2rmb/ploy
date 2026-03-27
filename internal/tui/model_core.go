package tui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
)

// InitialModel constructs the starting model for the TUI session.
func InitialModel(client *http.Client, baseURL *url.URL) model {
	ploy := newPloyList("PLOY", buildPloyItems(false, false, false))
	ploy.SetFilteringEnabled(false)

	return model{
		screen:    ScreenRoot,
		ploy:      ploy,
		secondary: newList("", nil),
		detail:    newList("", nil),
		client:    client,
		baseURL:   baseURL,
	}
}

// Init satisfies tea.Model. The base shell issues no async commands on start.
func (m model) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model and implements the six-screen navigation contract.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		case "esc":
			return m.handleEsc()
		}

	case tea.WindowSizeMsg:
		m.setWindowHeight(msg.Height)
		return m, nil

	case migsLoadedMsg:
		return m.handleMigsLoaded(msg), nil
	case runsLoadedMsg:
		return m.handleRunsLoaded(msg), nil
	case migDetailsLoadedMsg:
		return m.handleMigDetailsLoaded(msg), nil
	case runDetailsLoadedMsg:
		return m.handleRunDetailsLoaded(msg), nil
	case jobsLoadedMsg:
		return m.handleJobsLoaded(msg), nil
	}

	return m.updateActiveList(msg)
}

func (m model) handleMigsLoaded(msg migsLoadedMsg) model {
	sorted := make([]clitui.MigItem, len(msg.migs))
	copy(sorted, msg.migs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})
	items := make([]list.Item, len(sorted))
	for i, mig := range sorted {
		items[i] = listItem{
			title:       mig.Name,
			description: mig.ID.String(),
		}
	}
	m.secondary = newList("MIGRATIONS", items)
	m.applyWindowHeight()
	return m
}

func (m model) handleRunsLoaded(msg runsLoadedMsg) model {
	sorted := make([]runSummary, len(msg.runs))
	copy(sorted, msg.runs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	items := make([]list.Item, len(sorted))
	for i, run := range sorted {
		items[i] = listItem{
			title:       run.ID.String(),
			description: run.MigName + "  " + run.CreatedAt.Format("02 01 15:04"),
		}
	}
	m.runs = sorted
	m.secondary = newRunsList("RUNS", items)
	m.applyWindowHeight()
	return m
}

func (m model) handleMigDetailsLoaded(msg migDetailsLoadedMsg) model {
	if m.screen == ScreenMigrationDetails {
		m.ploy.SetItems(buildMigrationDetailsPloyItems(
			m.selectedMigName,
			m.selectedMigID,
			fmt.Sprintf("total: %d", msg.runTotal),
		))
		m.ploy.Select(0)
		return m
	}
	items := m.detail.Items()
	if len(items) >= 2 {
		items[0] = listItem{title: "repositories", description: fmt.Sprintf("total: %d", msg.repoTotal)}
		items[1] = listItem{title: "runs", description: fmt.Sprintf("total: %d", msg.runTotal)}
		m.detail.SetItems(items)
	}
	return m
}

func (m model) handleRunDetailsLoaded(msg runDetailsLoadedMsg) model {
	if m.screen == ScreenRunDetails {
		m.ploy.SetItems(buildRunDetailsPloyItems(
			m.selectedMigName,
			m.selectedMigID,
			m.selectedRunID,
			fmt.Sprintf("total: %d", msg.jobTotal),
		))
		m.ploy.Select(1)
		return m
	}
	items := m.detail.Items()
	if len(items) >= 2 {
		items[0] = listItem{title: "Repositories", description: fmt.Sprintf("total: %d", msg.repoTotal)}
		items[1] = listItem{title: "Jobs", description: fmt.Sprintf("total: %d", msg.jobTotal)}
		m.detail.SetItems(items)
	}
	return m
}

func (m model) handleJobsLoaded(msg jobsLoadedMsg) model {
	items := make([]list.Item, len(msg.jobs))
	for i, job := range msg.jobs {
		items[i] = listItem{
			title:       renderJobsPrimaryLine(job),
			description: renderJobsSecondaryLine(job),
		}
	}
	m.jobs = msg.jobs
	m.secondary = newJobsList("JOBS", items)
	m.applyWindowHeight()
	return m
}

func (m model) updateActiveList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case ScreenRoot:
		m.ploy, cmd = m.ploy.Update(msg)
	case ScreenMigrationsList:
		m.secondary, cmd = m.secondary.Update(msg)
	case ScreenMigrationDetails:
		m.ploy, cmd = m.ploy.Update(msg)
	case ScreenRunsList:
		m.secondary, cmd = m.secondary.Update(msg)
	case ScreenRunDetails:
		m.ploy, cmd = m.ploy.Update(msg)
	case ScreenJobsList:
		m.secondary, cmd = m.secondary.Update(msg)
	}
	return m, cmd
}
