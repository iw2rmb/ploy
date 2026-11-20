package tui

import (
	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"testing"
)

func makeRunDetailsModel(t *testing.T) model {
	t.Helper()
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-abc"}}})
	next, _ = next.(model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(model)
}

// Only the selected run's total updates its Jobs entry, preserving the cursor.
func TestRunDetailsLoadedUpdatesSelectedRunTotal(t *testing.T) {
	m := makeRunDetailsModel(t)
	m.rootList.Select(jobsIndex)
	next, _ := m.Update(runDetailsLoadedMsg{runID: "run-other", jobTotal: 99})
	m = next.(model)
	if m.runJobTotal != nil {
		t.Fatal("stale run total was accepted")
	}
	next, _ = m.Update(runDetailsLoadedMsg{runID: "run-abc", jobTotal: 7})
	m = next.(model)
	assertPloyItems(t, m, []listItem{{title: "Run", description: "run-abc"}, {title: "Jobs", description: "total: 7"}})
	if m.rootList.Index() != jobsIndex {
		t.Fatal("total response moved the cursor")
	}
}

// Scoped Jobs returns to the selected run and retains its latest total.
func TestRunJobsNavigationRestoresDetails(t *testing.T) {
	m := makeRunDetailsModel(t)
	m.rootList.Select(jobsIndex)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.screen != ScreenJobsList || m.selectedRunID != "run-abc" || cmd == nil {
		t.Fatalf("run Jobs = screen %v, scope %q", m.screen, m.selectedRunID)
	}
	next, _ = m.Update(runDetailsLoadedMsg{runID: "run-abc", jobTotal: 3})
	m = next.(model)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenRunDetails || cmd != nil || m.rootList.Index() != jobsIndex {
		t.Fatal("Esc failed to restore run details")
	}
	assertPloyItems(t, m, []listItem{{title: "Run", description: "run-abc"}, {title: "Jobs", description: "total: 3"}})
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenRunsList || !m.selectedRunID.IsZero() {
		t.Fatal("Esc failed to return to runs")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenPloyList {
		t.Fatal("Esc failed to return to root")
	}
}
