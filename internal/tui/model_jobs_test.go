package tui

import (
	"strings"
	"testing"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS6JobsListTitle verifies the JOBS secondary list title.
func TestS6JobsListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "alpha", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	if nm.secondary.Title != "JOBS" {
		t.Errorf("secondary list title: got %q, want %q", nm.secondary.Title, "JOBS")
	}
}

// TestS6JobsItemsPopulated verifies job rows use job name as title and include
// mig name, run id, and repo id in the description.
func TestS6JobsItemsPopulated(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{
			JobID:   domaintypes.JobID("job-abc"),
			Name:    "deploy",
			MigName: "my-mig",
			RunID:   domaintypes.RunID("run-xyz"),
			RepoID:  domaintypes.RepoID("repo-123"),
		},
	}})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 1 {
		t.Fatalf("secondary items: got %d, want 1", len(items))
	}

	item, ok := items[0].(listItem)
	if !ok {
		t.Fatalf("item 0: unexpected type %T", items[0])
	}
	if item.title != "deploy" {
		t.Errorf("item title: got %q, want %q", item.title, "deploy")
	}
	for _, want := range []string{"my-mig", "run-xyz", "repo-123"} {
		if !strings.Contains(item.description, want) {
			t.Errorf("item description %q: missing %q", item.description, want)
		}
	}
}

// TestS6JobsOrderingDeterministic verifies items are rendered in API order (no re-sorting).
func TestS6JobsOrderingDeterministic(t *testing.T) {
	m := InitialModel(nil, nil)
	jobs := []clitui.JobItem{
		{Name: "job-first", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
		{Name: "job-second", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
		{Name: "job-third", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
	}
	next, _ := m.Update(jobsLoadedMsg{jobs: jobs})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 3 {
		t.Fatalf("items count: got %d, want 3", len(items))
	}
	wantOrder := []string{"job-first", "job-second", "job-third"}
	for i, want := range wantOrder {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != want {
			t.Errorf("ordering: item %d title: got %q, want %q", i, item.title, want)
		}
	}
}

// TestS6EscTransitionsToS1 verifies Esc from S6 returns to S1.
func TestS6EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S6JobsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != S1Root {
		t.Errorf("Esc(S6): got screen %v, want S1Root", nm.screen)
	}
}

// TestS6ViewRendersSideBySide verifies that S6 view joins ploy and secondary lists.
func TestS6ViewRendersSideBySide(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S6JobsList
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	nm.screen = S6JobsList

	rendered := nm.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Error("view: missing PLOY list")
	}
	if !strings.Contains(rendered, "JOBS") {
		t.Error("view: missing JOBS list")
	}
}
