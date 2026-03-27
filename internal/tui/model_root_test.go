package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	clitui "github.com/iw2rmb/ploy/internal/client/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS1RootListTitle verifies the PLOY root list title.
func TestS1RootListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	if m.ploy.Title != "PLOY" {
		t.Errorf("root list title: got %q, want %q", m.ploy.Title, "PLOY")
	}
}

// TestS1RootItemDescriptions verifies the required detail lines for each root item.
func TestS1RootItemDescriptions(t *testing.T) {
	m := InitialModel(nil, nil)
	items := m.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("root items: got %d, want 3", len(items))
	}

	want := []struct{ title, desc string }{
		{"Migrations", "select migration"},
		{"Runs", "select run"},
		{"Jobs", "select job"},
	}
	for i, w := range want {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != w.title {
			t.Errorf("item %d title: got %q, want %q", i, item.title, w.title)
		}
		if item.description != w.desc {
			t.Errorf("item %d description: got %q, want %q", i, item.description, w.desc)
		}
	}
}

// TestS1FilteringDisabled verifies that filtering/search is disabled on the root list.
func TestS1FilteringDisabled(t *testing.T) {
	m := InitialModel(nil, nil)
	if m.ploy.FilteringEnabled() {
		t.Error("root list: filtering must be disabled")
	}
}

// TestS1EnterSelectsMigrations verifies Enter on index 0 transitions to S2.
func TestS1EnterSelectsMigrations(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(0)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != ScreenMigrationsList {
		t.Errorf("Enter(Migrations): got screen %v, want ScreenMigrationsList", nm.screen)
	}
}

// TestS1EnterSelectsRuns verifies Enter on index 1 transitions to S4.
func TestS1EnterSelectsRuns(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(1)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != ScreenRunsList {
		t.Errorf("Enter(Runs): got screen %v, want ScreenRunsList", nm.screen)
	}
}

// TestS1EnterSelectsJobs verifies Enter on index 2 transitions to S6.
func TestS1EnterSelectsJobs(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(2)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != ScreenJobsList {
		t.Errorf("Enter(Jobs): got screen %v, want ScreenJobsList", nm.screen)
	}
}

// TestS1EscQuits verifies Esc from root issues a quit command.
func TestS1EscQuits(t *testing.T) {
	m := InitialModel(nil, nil)
	_, cmd := m.handleEsc()
	if cmd == nil {
		t.Error("Esc from S1: expected quit cmd, got nil")
	}
}

// TestPloyListJobsSelectedShowsJobListPanel verifies that ScreenPloyList renders the
// JobList right-panel when the PLOY cursor is on the Jobs item (index 2).
func TestPloyListJobsSelectedShowsJobListPanel(t *testing.T) {
	m := InitialModel(nil, nil)
	// Populate jobs so the panel has content to render.
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)
	// Cursor must be on Jobs (index 2) while remaining on ScreenPloyList.
	m.ploy.Select(2)

	rendered := m.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Error("view: missing PLOY list")
	}
	if !strings.Contains(rendered, "JOBS") {
		t.Error("view: missing JOBS panel when Jobs item selected on ScreenPloyList")
	}
}

// TestPloyListNonJobsSelectedShowsOnlyPloy verifies that ScreenPloyList renders
// only the PLOY list when the cursor is not on the Jobs item.
func TestPloyListNonJobsSelectedShowsOnlyPloy(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{"Migrations", 0},
		{"Runs", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.ploy.Select(tt.index)

			rendered := m.View().Content
			if !strings.Contains(rendered, "PLOY") {
				t.Error("view: missing PLOY list")
			}
			if strings.Contains(rendered, "JOBS") {
				t.Errorf("view: unexpected JOBS panel when cursor is on %s (index %d)", tt.name, tt.index)
			}
		})
	}
}

// TestPloyListFocusRemainsOnPloy verifies that ScreenPloyList routes key messages
// to the PLOY list, not the JobList, keeping PLOY as the active list.
// It sends a real "k" (up) key while the ploy cursor is on Jobs (index 2) and
// confirms the ploy cursor moves while the jobList index remains unchanged.
func TestPloyListFocusRemainsOnPloy(t *testing.T) {
	m := InitialModel(nil, nil)

	// Load two jobs so the jobList has multiple items to potentially navigate.
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "alpha", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
		{JobID: domaintypes.JobID("job-2"), Name: "beta", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)

	// Place ploy cursor on Jobs (index 2) — the jobs panel is visible.
	m.ploy.Select(2)
	jobListIdxBefore := m.jobList.Index()

	// Send a real "k" (up) key through Update; on ScreenPloyList this must
	// be routed to m.ploy, not m.jobList.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(model)

	// ploy cursor must have moved up (from 2 → 1).
	if m.ploy.Index() != 1 {
		t.Errorf("ploy cursor: got %d, want 1 after 'k' key on ScreenPloyList", m.ploy.Index())
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
