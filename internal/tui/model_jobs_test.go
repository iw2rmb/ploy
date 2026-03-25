package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

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
	if nm.secondary.Width() != jobsListWidth {
		t.Errorf("secondary list width: got %d, want %d", nm.secondary.Width(), jobsListWidth)
	}
}

// TestS6JobsItemsPopulated verifies the two-line job row contract.
func TestS6JobsItemsPopulated(t *testing.T) {
	m := InitialModel(nil, nil)
	nodeID := domaintypes.NodeID("abc123")
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{
			JobID:      domaintypes.JobID("job-abc"),
			Name:       "deploy",
			Status:     domaintypes.JobStatusSuccess,
			DurationMs: 2500,
			JobImage:   "ghcr.io/iw2rmb/ploy/migs-java17:latest",
			NodeID:     &nodeID,
			MigName:    "my-mig",
			RunID:      domaintypes.RunID("run-xyz"),
			RepoID:     domaintypes.RepoID("repo-123"),
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
	if !strings.Contains(item.title, "✓ deploy") {
		t.Errorf("item title: expected status glyph and name, got %q", item.title)
	}
	if !strings.HasSuffix(strings.TrimSpace(item.title), "2.5s") {
		t.Errorf("item title: expected duration suffix %q, got %q", "2.5s", item.title)
	}
	durationCell := string([]rune(item.title)[jobsListWidth-jobsDurationWidth : jobsListWidth])
	if want := "      2.5s"; durationCell != want {
		t.Errorf("item duration cell: got %q, want %q", durationCell, want)
	}
	if got := utf8.RuneCountInString(item.title); got != jobsListWidth {
		t.Errorf("item title rune width: got %d, want %d", got, jobsListWidth)
	}
	if want := "job-abc"; item.description != want {
		t.Errorf("item description: got %q, want %q", item.description, want)
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
		if !strings.Contains(item.title, want) {
			t.Errorf("ordering: item %d title %q missing job name %q", i, item.title, want)
		}
	}
}

// TestS6EscTransitionsToS1 verifies Esc from S6 returns to S1.
func TestS6EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenRoot {
		t.Errorf("Esc(S6): got screen %v, want ScreenRoot", nm.screen)
	}
}

// TestS6ViewRendersSideBySide verifies that S6 view joins ploy and secondary lists.
func TestS6ViewRendersSideBySide(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{Name: "deploy", Status: domaintypes.JobStatusRunning, JobID: domaintypes.JobID("job-1"), MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	nm.screen = ScreenJobsList

	rendered := nm.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Error("view: missing PLOY list")
	}
	if !strings.Contains(rendered, "JOBS") {
		t.Error("view: missing JOBS list")
	}
	if !strings.Contains(rendered, "job-1") {
		t.Error("view: missing jobs secondary row format")
	}
}

func TestS6EnterDefinesAllPloyItems(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	items := rm.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy items: got %d, want 3", len(items))
	}

	item0, ok := items[0].(listItem)
	if !ok {
		t.Fatalf("item 0: unexpected type %T", items[0])
	}
	item1, ok := items[1].(listItem)
	if !ok {
		t.Fatalf("item 1: unexpected type %T", items[1])
	}
	item2, ok := items[2].(listItem)
	if !ok {
		t.Fatalf("item 2: unexpected type %T", items[2])
	}

	if item0.title != "Migration" {
		t.Errorf("item 0 title: got %q, want %q", item0.title, "Migration")
	}
	if item1.title != "Run" {
		t.Errorf("item 1 title: got %q, want %q", item1.title, "Run")
	}
	if item2.title != "Job" {
		t.Errorf("item 2 title: got %q, want %q", item2.title, "Job")
	}
}
