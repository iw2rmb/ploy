package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

var quitKey = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit"))

func InitialModel(ctx context.Context, client *http.Client, baseURL *url.URL) model {
	ploy := newPloyList("PLOY", buildPloyItems(false, "", nil))
	ploy.SetFilteringEnabled(false)
	return model{
		screen:        ScreenPloyList,
		rootList:      ploy,
		rightPaneList: newRunsList("RUNS", nil),
		jobList:       joblist.New("JOBS"),
		usersList:     newListWithWidth("USERS", nil, runsListWidth),
		usersLoading:  true,
		ctx:           ctx,
		client:        client,
		baseURL:       baseURL,
		now:           time.Now(),
		// Reserve the startup request while the token capability determines its creator scope.
		runsRequest: 1,
		runsLoading: true,
	}
}

func (m model) Init() tea.Cmd {
	return loadUsersCmd(m.ctx, m.client, m.baseURL)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok && key.Matches(press, quitKey) {
		return m, tea.Quit
	}
	next, cmd := m.update(msg)
	m = next.(model)
	refresh := m.scheduleRefresh()
	return m, tea.Batch(cmd, refresh)
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.settingFilter() || (msg.String() == "esc" && m.isFiltered()) {
			return m.updateActiveList(msg)
		}
		switch msg.String() {
		case "enter":
			return m.handleEnter()
		case "esc":
			return m.handleEsc()
		}
	case refreshTickMsg:
		return m.handleRefreshTick(time.Time(msg))
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.windowWidth = msg.Width
		}
		if msg.Height > 0 {
			m.windowHeight = msg.Height
		}
		m.applyWindowSize()
		return m, nil
	case usersLoadedMsg:
		return m.handleUsersLoaded(msg)
	case runsLoadedMsg:
		return m.handleRunsLoaded(msg), nil
	case runDetailsLoadedMsg:
		return m.handleRunDetailsLoaded(msg), nil
	case jobsLoadedMsg:
		return m.handleJobsLoaded(msg), nil
	case errMsg:
		m.lastErr = msg.err
		return m, nil
	}
	return m.updateActiveList(msg)
}

func (m model) settingFilter() bool {
	switch m.screen {
	case ScreenRunsList:
		return m.rightPaneList.SettingFilter()
	case ScreenJobsList:
		return m.jobList.SettingFilter()
	case ScreenUsersList:
		return m.usersList.SettingFilter()
	}
	return false
}

func (m model) isFiltered() bool {
	switch m.screen {
	case ScreenRunsList:
		return m.rightPaneList.IsFiltered()
	case ScreenJobsList:
		return m.jobList.IsFiltered()
	case ScreenUsersList:
		return m.usersList.IsFiltered()
	}
	return false
}

func (m model) handleRunsLoaded(msg runsLoadedMsg) model {
	if msg.requestID != m.runsRequest {
		return m
	}
	m.runsLoading = false
	if msg.err != nil {
		m.lastErr = msg.err
		return m
	}
	items := make([]list.Item, len(msg.runs))
	for i, run := range msg.runs {
		items[i] = runItem{run: run}
		if run.ID == m.selectedRunID && !run.ID.IsZero() {
			m.rootList.SetItem(runsIndex, items[i])
		}
	}
	selectedID := domaintypes.RunID("")
	if selected, ok := m.rightPaneList.SelectedItem().(runItem); ok {
		selectedID = selected.run.ID
	}
	m.rightPaneList.SetItems(items)
	m.refilterRuns()
	for index, item := range m.rightPaneList.VisibleItems() {
		if item.(runItem).run.ID == selectedID {
			m.rightPaneList.Select(index)
			break
		}
	}
	m.applyWindowSize()
	return m
}

func (m *model) refilterRuns() {
	state := m.rightPaneList.FilterState()
	if state == list.Unfiltered {
		return
	}
	input := m.rightPaneList.FilterInput
	selectedID := domaintypes.RunID("")
	if selected, ok := m.rightPaneList.SelectedItem().(runItem); ok {
		selectedID = selected.run.ID
	}
	m.rightPaneList.SetFilterText(input.Value())
	m.rightPaneList.SetFilterState(state)
	m.rightPaneList.FilterInput = input
	for index, item := range m.rightPaneList.VisibleItems() {
		if item.(runItem).run.ID == selectedID {
			m.rightPaneList.Select(index)
			break
		}
	}
}

func (m model) handleRunDetailsLoaded(msg runDetailsLoadedMsg) model {
	if msg.runID != m.selectedRunID || msg.runID.IsZero() {
		return m
	}
	m.runJobTotal = &msg.jobTotal
	if m.screen == ScreenRunDetails {
		m.rootList.SetItem(jobsIndex, m.runJobsEntry())
	}
	return m
}

func (m model) runJobsEntry() listItem {
	total := "total: —"
	if m.runJobTotal != nil {
		total = fmt.Sprintf("total: %d", *m.runJobTotal)
	}
	return listItem{title: "Jobs", description: total}
}

func (m model) handleJobsLoaded(msg jobsLoadedMsg) model {
	if msg.requestID != m.jobsRequest {
		return m
	}
	m.jobsLoading = false
	if msg.runID != m.selectedRunID {
		return m
	}
	if msg.err != nil {
		m.lastErr = msg.err
		if msg.jobs == nil {
			return m
		}
	}
	m.now = time.Now()
	hadSelection := !m.jobList.ConfirmedJobID().IsZero()
	m.jobList = m.jobList.RefreshJobs(msg.jobs)
	if hadSelection {
		if item, ok := m.jobList.ConfirmedItem(m.now, m.spinnerFrame); ok {
			m.rootList.SetItem(jobsIndex, item)
		} else if m.selectedRunID.IsZero() {
			m.resetRoot()
		} else {
			m.rootList.SetItem(jobsIndex, m.runJobsEntry())
		}
	}
	m.applyWindowSize()
	return m
}

func (m model) updateActiveList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case ScreenPloyList:
		prevIdx := m.rootList.Index()
		m.rootList, cmd = m.rootList.Update(msg)
		if m.rootList.Index() != prevIdx {
			cmd = tea.Batch(cmd, m.loadRootPreview())
		}
	case ScreenRunDetails:
		m.rootList, cmd = m.rootList.Update(msg)
	case ScreenRunsList:
		if _, ok := msg.(list.FilterMatchesMsg); ok {
			// Filter results carry row snapshots that may predate a periodic refresh.
			m.refilterRuns()
			return m, nil
		}
		m.rightPaneList, cmd = m.rightPaneList.Update(msg)
	case ScreenJobsList:
		m.jobList, cmd = m.jobList.Update(msg)
	case ScreenUsersList:
		if _, ok := msg.(list.FilterMatchesMsg); ok {
			// Filter commands from another list can finish after focus changes.
			state, input := m.usersList.FilterState(), m.usersList.FilterInput
			selected, selectedOK := m.usersList.SelectedItem().(userItem)
			if state != list.Unfiltered {
				m.usersList.SetFilterText(input.Value())
				m.usersList.SetFilterState(state)
				m.usersList.FilterInput = input
				if selectedOK {
					for index, item := range m.usersList.VisibleItems() {
						if item.(userItem).username == selected.username {
							m.usersList.Select(index)
							break
						}
					}
				}
			}
			return m, nil
		}
		m.usersList, cmd = m.usersList.Update(msg)
	}
	return m, cmd
}
