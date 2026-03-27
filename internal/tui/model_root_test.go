package tui

import (
	"strings"
	"testing"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
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
func TestPloyListFocusRemainsOnPloy(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(0)
	// Route a navigation message through updateActiveList.
	_, _ = m.updateActiveList(nil)
	// The screen must remain ScreenPloyList with ploy as active list.
	if m.screen != ScreenPloyList {
		t.Errorf("screen changed: got %v, want ScreenPloyList", m.screen)
	}
}
