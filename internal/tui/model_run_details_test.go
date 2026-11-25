package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func makeRunDetailsModel(t *testing.T) model {
	t.Helper()
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenRunsList
	creator := "alice"
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "run-abc", SpecName: "java", Status: domaintypes.RunStatusSuccess, RepoURL: "https://example.com/team/app.git", CreatedBy: &creator}}})
	next, _ = next.(model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(model)
}

// Backspace on the confirmed run restores PLOY and preserves the filtered RUNS cursor.
func TestBackspaceClearsConfirmedRunInPloy(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{{ID: "first"}, {ID: "second"}}})
	m = next.(model)
	m.rightPaneList.SetFilterText("second")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ = next.(model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.selectedRunID != "second" || m.screen != ScreenRunDetails {
		t.Fatal("filtered run was not confirmed")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(model)
	if !m.selectedRunID.IsZero() || m.screen != ScreenPloyList || m.rootList.Index() != runsIndex {
		t.Fatal("Backspace did not return to root Runs and clear the confirmed run")
	}
	if item, ok := m.visibleRun(); !ok || item.run.ID != "second" || m.rightPaneList.FilterValue() != "second" {
		t.Fatal("clearing the confirmed run lost the filtered preview")
	}
}

// Only the selected run's total updates its Jobs entry, preserving the cursor.
func TestRunDetailsLoadedUpdatesSelectedRunTotal(t *testing.T) {
	m := makeRunDetailsModel(t)
	m.rootList.Select(jobsIndex)
	next, _ := m.Update(runDetailsLoadedMsg{requestID: m.detailsRequest, runDetails: runDetails{runID: "run-other", jobTotal: new(99)}})
	m = next.(model)
	if m.runDetails.jobTotal != nil {
		t.Fatal("stale run total was accepted")
	}
	next, _ = m.Update(runDetailsLoadedMsg{requestID: m.detailsRequest, runDetails: runDetails{runID: "run-abc", jobTotal: new(7)}})
	m = next.(model)
	assertPloyItems(t, m, []listItem{{title: "✓ java", description: "team/app\n@alice"}, {title: "Jobs", description: "total: 7\n"}})
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
	next, _ = m.Update(runDetailsLoadedMsg{requestID: m.detailsRequest, runDetails: runDetails{runID: "run-abc", jobTotal: new(3)}})
	m = next.(model)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenRunDetails || cmd != nil || m.rootList.Index() != jobsIndex {
		t.Fatal("Esc failed to restore run details")
	}
	assertPloyItems(t, m, []listItem{{title: "✓ java", description: "team/app\n@alice"}, {title: "Jobs", description: "total: 3\n"}})
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

// Unassigned active runs retry on refresh, and a new attempt rejects the previous lookup.
func TestUnassignedNodeRetriesAndRejectsStaleAttempt(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	run := domaintypes.RunSummary{ID: "run-1", Attempt: 1, Status: domaintypes.RunStatusRunning}
	refresh := func() {
		next, _ := m.Update(runsLoadedMsg{requestID: m.runsRequest, runs: []domaintypes.RunSummary{run}})
		m = next.(model)
	}
	refresh()
	reply := runDetailsLoadedMsg{runDetails: m.runDetails, requestID: m.detailsRequest}
	reply.jobTotal = new(1)
	next, _ := m.Update(reply)
	m = next.(model)
	refresh()
	if !m.detailsLoading || m.detailsRequest != reply.requestID+1 {
		t.Fatal("unassigned run did not retry")
	}
	reply.requestID = m.detailsRequest
	reply.nodeName = "alpha"
	run.Attempt++
	refresh()
	next, _ = m.Update(reply)
	m = next.(model)
	if m.runDetails.attempt != 2 || m.runDetails.nodeName != "" || !m.detailsLoading {
		t.Fatal("a late lookup from the first attempt replaced the current lookup")
	}
	reply.runDetails, reply.requestID = m.runDetails, m.detailsRequest
	reply.jobTotal, reply.nodeName = new(2), "beta"
	next, _ = m.Update(reply)
	m = next.(model)
	refresh()
	if m.runDetails.nodeName != "beta" || m.detailsLoading || m.detailsRequest != reply.requestID {
		t.Fatal("refresh discarded the assigned node for the current attempt")
	}
}
