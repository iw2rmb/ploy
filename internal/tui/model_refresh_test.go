package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Active rows animate every second, with one request at a time, until terminal data arrives.
func TestRunsRefreshLifecycle(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	next, cmd := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-1", Status: domaintypes.RunStatusRunning}}})
	m = next.(model)
	if cmd == nil || !m.refreshScheduled {
		t.Fatal("active runs did not schedule a tick")
	}
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	next, cmd = m.Update(refreshTickMsg(now))
	m = next.(model)
	if cmd == nil || !m.runsLoading || !m.refreshScheduled || m.runsRequest != 2 || m.spinnerFrame != 1 || !m.now.Equal(now) {
		t.Fatalf("tick did not animate and load: %+v", m)
	}
	next, _ = m.Update(refreshTickMsg(now.Add(time.Second)))
	m = next.(model)
	if m.runsRequest != 2 || m.spinnerFrame != 2 {
		t.Fatal("pending request blocked animation or started an overlapping load")
	}
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-1", Status: domaintypes.RunStatusSuccess}}})
	m = next.(model)
	if m.runsLoading {
		t.Fatal("completed request stayed in flight")
	}
	next, cmd = m.Update(refreshTickMsg(now.Add(2 * time.Second)))
	m = next.(model)
	if cmd != nil || m.refreshScheduled || m.runsRequest != 2 {
		t.Fatal("terminal rows continued polling")
	}
}

// A late filter response must not restore a run removed by the latest API response.
func TestRunsRefreshRejectsOldFilterRows(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-old"}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(model)
	next, filterCmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "run"})
	m = next.(model)
	oldFilter := findFilterResult(t, filterCmd)
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-current"}}})
	m = next.(model)
	next, _ = m.Update(oldFilter)
	m = next.(model)
	if m.rightPaneList.SelectedItem().(runItem).run.ID != "run-current" || m.rightPaneList.FilterValue() != "run" || !m.rightPaneList.SettingFilter() {
		t.Fatal("late filter restored stale rows or changed the filter")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.(model).selectedRunID != "run-current" {
		t.Fatal("Enter selected a removed run")
	}
}

func findFilterResult(t *testing.T, cmd tea.Cmd) list.FilterMatchesMsg {
	t.Helper()
	var result list.FilterMatchesMsg
	var visit func(tea.Cmd)
	visit = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case list.FilterMatchesMsg:
			result = msg
		case tea.BatchMsg:
			for _, child := range msg {
				visit(child)
			}
		}
	}
	visit(cmd)
	if len(result) == 0 {
		t.Fatal("filter command returned no matching rows")
	}
	return result
}

// A running job loaded after idle time immediately uses the current clock.
func TestJobsLoadedUpdatesElapsedClock(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenJobsList
	m.now = time.Now().Add(-24 * time.Hour)
	started := time.Now().Add(-time.Hour)
	before := time.Now()
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{{JobID: "job-1", Status: domaintypes.JobStatusRunning, StartedAt: &started}}})
	m = next.(model)
	if m.now.Before(before) || !strings.Contains(m.View().Content, "1h    ") {
		t.Fatal("loaded job used the old idle clock")
	}
}

// The inactive Jobs preview refreshes, and leaving it prevents the pending tick from polling hidden data.
func TestJobsPreviewRefreshFollowsVisibilityAndRetries(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.rootList.Select(jobsIndex)
	next, cmd := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{{JobID: "job-1", Status: domaintypes.JobStatusQueued}}})
	m = next.(model)
	if cmd == nil || !m.refreshScheduled {
		t.Fatal("queued preview did not schedule refresh")
	}
	next, _ = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if !m.jobsLoading || m.jobsRequest != 1 {
		t.Fatal("preview did not load")
	}
	next, _ = m.Update(jobsLoadedMsg{requestID: m.jobsRequest, err: errors.New("temporary failure")})
	m = next.(model)
	if m.jobsLoading || !m.refreshScheduled {
		t.Fatal("failed request prevented retry")
	}
	next, _ = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if m.jobsRequest != 2 {
		t.Fatal("active preview did not retry")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(model)
	next, cmd = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if cmd != nil || m.refreshScheduled || m.jobsRequest != 2 {
		t.Fatal("hidden jobs continued polling")
	}
	next, _ = m.Update(jobsLoadedMsg{requestID: m.jobsRequest, jobs: []domainapi.JobListItem{{JobID: "job-1", Status: domaintypes.JobStatusRunning}}})
	m = next.(model)
	if m.jobsLoading || m.refreshScheduled {
		t.Fatal("hidden response retained loading or restarted timer")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	if cmd == nil || !m.refreshScheduled {
		t.Fatal("exposing active cached jobs did not restart timer")
	}
}

// Run refresh retains filter editing state and the selected identity when API order changes.
func TestRunsRefreshPreservesFilterAndSelection(t *testing.T) {
	for _, state := range []list.FilterState{list.Filtering, list.FilterApplied} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		m.screen = ScreenRunsList
		next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-first"}, {ID: "run-second"}}})
		m = next.(model)
		m.rightPaneList.SetFilterText("run")
		m.rightPaneList.SetFilterState(state)
		m.rightPaneList.Select(1)
		next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-second"}, {ID: "run-first"}}})
		m = next.(model)
		if m.rightPaneList.FilterValue() != "run" || m.rightPaneList.FilterState() != state || m.rightPaneList.SelectedItem().(runItem).run.ID != "run-second" {
			t.Fatal("refresh lost the filter or selected identity")
		}
	}
}

// The confirmed run refreshes while its row is highlighted in PLOY.
func TestRunsRefreshResumesAfterDetails(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-1", Status: domaintypes.RunStatusRunning}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	next, cmd := m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if cmd == nil || !m.runsLoading {
		t.Fatal("visible confirmed run did not refresh")
	}
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-1", Status: domaintypes.RunStatusRunning, SourceCommitSHA: "12345678"}}})
	m = next.(model)
	if item, ok := m.visibleRun(); !ok || item.run.SourceCommitSHA != "12345678" {
		t.Fatal("confirmed run retained its old summary")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, cmd = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if cmd != nil || m.refreshScheduled || m.runsLoading {
		t.Fatal("highlighting Jobs continued polling hidden runs")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if cmd == nil || !m.refreshScheduled {
		t.Fatal("returning to active runs did not schedule refresh")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(model)
	if cmd != nil || !m.refreshScheduled {
		t.Fatal("unrelated input changed the pending timer")
	}
}

// Hidden startup results update cached runs; returning to the preview resumes guarded refresh.
func TestRunsPreviewRefreshFollowsVisibility(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	startupRequest := m.runsRequest
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, cmd := m.Update(runsLoadedMsg{requestID: startupRequest, runs: []domaintypes.RunSummary{{ID: "run-active", Status: domaintypes.RunStatusRunning}}})
	m = next.(model)
	if cmd != nil || m.runsLoading || m.refreshScheduled || len(m.rightPaneList.Items()) != 1 {
		t.Fatal("hidden startup result was discarded or started hidden polling")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(model)
	if cmd == nil || !m.refreshScheduled || !m.runsLoading || m.runsRequest != startupRequest+1 || m.screen != ScreenPloyList {
		t.Fatal("root Runs revisit did not refresh its visible preview")
	}
	next, _ = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if m.runsRequest != startupRequest+1 || m.spinnerFrame != 1 || !m.refreshScheduled {
		t.Fatal("preview tick overlapped its pending request or stopped animation")
	}
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-active", Status: domaintypes.RunStatusRunning}}})
	m = next.(model)
	next, _ = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if m.runsRequest != startupRequest+2 || !m.runsLoading {
		t.Fatal("visible Runs preview did not poll after its prior request finished")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	next, cmd = m.Update(refreshTickMsg(time.Now()))
	m = next.(model)
	if cmd != nil || m.refreshScheduled || m.runsRequest != startupRequest+2 {
		t.Fatal("hidden Runs preview continued polling")
	}
	next, cmd = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-active", Status: domaintypes.RunStatusRunning}}})
	m = next.(model)
	if cmd != nil || m.runsLoading || m.refreshScheduled {
		t.Fatal("late hidden Runs response restarted polling")
	}
}
