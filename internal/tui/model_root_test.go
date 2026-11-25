package tui

import (
	"errors"

	"charm.land/bubbles/v2/list"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRootListContract(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	if m.rootList.Title != "PLOY" {
		t.Errorf("root list title: got %q, want %q", m.rootList.Title, "PLOY")
	}
	if m.rootList.Width() != ployListWidth {
		t.Errorf("root list width: got %d, want %d", m.rootList.Width(), ployListWidth)
	}
	if m.rootList.FilteringEnabled() {
		t.Error("root list filtering must be disabled")
	}
	assertPloyItems(t, m, []listItem{
		{title: "Runs", description: "select run\n"},
		{title: "Jobs", description: "select job\n"},
		{title: "Users", description: "cli-admin only\n"},
	})
}

// TestPloyListJobsSelectedShowsJobListPanel verifies that ScreenPloyList renders the
// JobList right-panel when the PLOY cursor is on the Jobs item (index 1).
func TestPloyListJobsSelectedShowsJobListPanel(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	// Populate jobs so the panel has content to render.
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), DisplayName: "deploy", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)
	// Cursor must be on Jobs (index 1) while remaining on ScreenPloyList.
	m.rootList.Select(1)

	rendered := m.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Error("view: missing PLOY list")
	}
	if !strings.Contains(rendered, "JOBS") {
		t.Error("view: missing JOBS panel when Jobs item selected on ScreenPloyList")
	}
}

// Root Runs renders the existing list while PLOY retains focus.
func TestPloyListRunsSelectedShowsRunsPanel(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = next.(model)
	next, _ = m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-id", SpecID: "spec-preview"}}})
	m = next.(model)
	view := m.View().Content
	for _, want := range []string{"PLOY", "RUNS", "spec-preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("root Runs preview missing %q", want)
		}
	}
	if strings.Contains(view, "JOBS") || m.screen != ScreenPloyList {
		t.Fatal("root Runs showed another pane or transferred focus")
	}
}

// TestPloyListFocusRemainsOnPloy verifies that ScreenPloyList routes key messages
// to the PLOY list, not the JobList, keeping PLOY as the active list.
// It sends a real "k" (up) key while the ploy cursor is on Jobs (index 1) and
// confirms the ploy cursor moves while the jobList index remains unchanged.
func TestPloyListFocusRemainsOnPloy(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false

	// Load two jobs so the jobList has multiple items to potentially navigate.
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), DisplayName: "alpha", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
		{JobID: domaintypes.JobID("job-2"), DisplayName: "beta", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)

	// Place ploy cursor on Jobs (index 1) — the jobs panel is visible.
	m.rootList.Select(1)
	jobListIdxBefore := m.jobList.Index()

	// Send a real "k" (up) key through Update; on ScreenPloyList this must
	// be routed to m.rootList, not m.jobList.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(model)

	// ploy cursor must have moved up (from 1 → 0).
	if m.rootList.Index() != 0 {
		t.Errorf("ploy cursor: got %d, want 0 after 'k' key on ScreenPloyList", m.rootList.Index())
	}
	// jobList cursor must be untouched.
	if m.jobList.Index() != jobListIdxBefore {
		t.Errorf("jobList cursor: got %d, want %d — jobList must not receive keys on ScreenPloyList",
			m.jobList.Index(), jobListIdxBefore)
	}
	// Screen must remain ScreenPloyList.
	if m.screen != ScreenPloyList {
		t.Errorf("screen changed: got %v, want ScreenPloyList", m.screen)
	}
}

// Enter transfers focus without duplicating either preview's outstanding request.
func TestRootEnterReusesPendingPreviewRequest(t *testing.T) {
	for _, index := range []int{runsIndex, jobsIndex} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		want := ScreenRunsList
		if index == jobsIndex {
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m = next.(model)
			want = ScreenJobsList
		}
		runsRequest, jobsRequest := m.runsRequest, m.jobsRequest
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(model)
		if cmd != nil || m.screen != want || m.runsRequest != runsRequest || m.jobsRequest != jobsRequest {
			t.Fatalf("Enter restarted pending preview %d", index)
		}
		if index == runsIndex && !m.runsLoading || index == jobsIndex && !m.jobsLoading {
			t.Fatalf("Enter lost pending request %d", index)
		}
	}
}

// Enter preserves cached rows, filtering, and cursor position in either preview.
func TestRootEnterPreservesPreviewListState(t *testing.T) {
	for _, index := range []int{runsIndex, jobsIndex} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		if index == runsIndex {
			next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-first"}, {ID: "run-second"}}})
			m = next.(model)
			m.rightPaneList.SetFilterText("run")
			m.rightPaneList.SetFilterState(list.FilterApplied)
			m.rightPaneList.Select(1)
			m.syncRunDetails(false, false)
		} else {
			m.screen = ScreenJobsList
			m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "job-first", DisplayName: "step"}, {JobID: "job-second", DisplayName: "step"}})
			for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 's', Text: "step"}, {Code: tea.KeyEnter}} {
				next, cmd := m.Update(key)
				m = applyRunFilterCommands(next.(model), cmd)
			}
			m.jobList = m.jobList.Select(1)
			m.screen = ScreenPloyList
			m.rootList.Select(jobsIndex)
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(model)
		if cmd != nil {
			t.Fatalf("Enter reloaded preview %d", index)
		}
		if index == runsIndex {
			if len(m.rightPaneList.Items()) != 2 || !m.rightPaneList.IsFiltered() || m.rightPaneList.SelectedItem().(runItem).run.ID != "run-second" {
				t.Fatal("Enter reset Runs rows, filter, or cursor")
			}
		} else if len(m.jobList.Jobs()) != 2 || !m.jobList.IsFiltered() || m.jobList.SelectedJobID() != "job-second" {
			t.Fatal("Enter reset Jobs rows, filter, or cursor")
		}
	}
}

// Empty results and failed requests can retry on a later root selection, without overlapping loads.
func TestRootPreviewRetriesOnRevisit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		result := runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{}}
		if fail {
			result.err = errors.New("temporary failure")
		}
		next, _ := m.Update(result)
		m = next.(model)
		before := m.runsRequest
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(model)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		m = next.(model)
		if cmd == nil || !m.runsLoading || m.runsRequest != before+1 {
			t.Fatal("root Runs revisit did not retry")
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(model)
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		m = next.(model)
		if m.runsRequest != before+1 {
			t.Fatal("root Runs revisit overlapped its pending request")
		}
	}
}

// Esc returns the loaded list to preview mode without clearing its cursor or rows.
func TestRootPreviewSurvivesEnterAndEsc(t *testing.T) {
	for _, index := range []int{runsIndex, jobsIndex} {
		m := InitialModel(t.Context(), nil, nil)
		m.usersLoading = false
		m.rootList.Select(index)
		if index == runsIndex {
			next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "first"}, {ID: "second"}}})
			m = next.(model)
			m.rightPaneList.Select(1)
			m.syncRunDetails(false, false)
		} else {
			m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "first"}, {JobID: "second"}}).Select(1)
		}
		for _, key := range []rune{tea.KeyEnter, tea.KeyEscape} {
			next, cmd := m.Update(tea.KeyPressMsg{Code: key})
			m = next.(model)
			if cmd != nil {
				t.Fatal("focus transition reloaded a populated preview")
			}
		}
		if m.screen != ScreenPloyList || m.rootList.Index() != index {
			t.Fatal("Esc did not return to its root entry")
		}
		if index == runsIndex && (len(m.rightPaneList.Items()) != 2 || m.rightPaneList.Index() != 1) || index == jobsIndex && (len(m.jobList.Jobs()) != 2 || m.jobList.Index() != 1) {
			t.Fatal("focus transition reset list rows or cursor")
		}
	}
}
