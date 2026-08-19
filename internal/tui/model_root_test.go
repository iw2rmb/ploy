package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRootListContract(t *testing.T) {
	m := InitialModel(nil, nil)
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
		{title: "Migrations", description: "select migration"},
		{title: "Runs", description: "select run"},
		{title: "Jobs", description: "select job"},
	})
}

// TestPloyListJobsSelectedShowsJobListPanel verifies that ScreenPloyList renders the
// JobList right-panel when the PLOY cursor is on the Jobs item (index 2).
func TestPloyListJobsSelectedShowsJobListPanel(t *testing.T) {
	m := InitialModel(nil, nil)
	// Populate jobs so the panel has content to render.
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)
	// Cursor must be on Jobs (index 2) while remaining on ScreenPloyList.
	m.rootList.Select(2)

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
			m.rootList.Select(tt.index)

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
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), Name: "alpha", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
		{JobID: domaintypes.JobID("job-2"), Name: "beta", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)

	// Place ploy cursor on Jobs (index 2) — the jobs panel is visible.
	m.rootList.Select(2)
	jobListIdxBefore := m.jobList.Index()

	// Send a real "k" (up) key through Update; on ScreenPloyList this must
	// be routed to m.rootList, not m.jobList.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(model)

	// ploy cursor must have moved up (from 2 → 1).
	if m.rootList.Index() != 1 {
		t.Errorf("ploy cursor: got %d, want 1 after 'k' key on ScreenPloyList", m.rootList.Index())
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
