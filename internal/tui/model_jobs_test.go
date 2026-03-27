package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

// defaultItem is a local interface matching the title/description contract
// implemented by all list rows built by the joblist component.
type defaultItem interface {
	Title() string
	Description() string
}

func mustDefaultItem(t *testing.T, v interface{}) defaultItem {
	t.Helper()
	di, ok := v.(defaultItem)
	if !ok {
		t.Fatalf("item does not implement defaultItem: %T", v)
	}
	return di
}

// TestS6JobsListTitle verifies the JOBS jobList title and width.
func TestS6JobsListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "alpha", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	if nm.jobList.Title() != "JOBS" {
		t.Errorf("jobList title: got %q, want %q", nm.jobList.Title(), "JOBS")
	}
	if nm.jobList.Width() != joblist.ListWidth {
		t.Errorf("jobList width: got %d, want %d", nm.jobList.Width(), joblist.ListWidth)
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

	items := nm.jobList.Items()
	if len(items) != 1 {
		t.Fatalf("jobList items: got %d, want 1", len(items))
	}

	row := mustDefaultItem(t, items[0])
	title := row.Title()
	desc := row.Description()

	if !strings.Contains(title, "⏺") || !strings.Contains(title, "deploy") {
		t.Errorf("item title: expected status glyph and name, got %q", title)
	}
	if !strings.Contains(title, "\x1b[") {
		t.Errorf("item title: expected ANSI color sequence for terminal status glyph, got %q", title)
	}
	if !strings.HasSuffix(strings.TrimSpace(title), "2s") {
		t.Errorf("item title: expected duration suffix %q, got %q", "2s", title)
	}
	if !strings.HasSuffix(title, " 2s") {
		t.Errorf("item title: expected right-aligned duration with spacing, got %q", title)
	}
	if got := lipgloss.Width(title); got != joblist.ContentWidth {
		t.Errorf("item title visible width: got %d, want %d", got, joblist.ContentWidth)
	}
	if strings.Contains(title, "...") || strings.Contains(title, "…") {
		t.Errorf("item title: duration/name must not be ellipsized, got %q", title)
	}
	if want := "job-abc"; desc != want {
		t.Errorf("item description: got %q, want %q", desc, want)
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

	items := nm.jobList.Items()
	if len(items) != 3 {
		t.Fatalf("items count: got %d, want 3", len(items))
	}
	wantOrder := []string{"job-first", "job-second", "job-third"}
	for i, want := range wantOrder {
		row := mustDefaultItem(t, items[i])
		if !strings.Contains(row.Title(), want) {
			t.Errorf("ordering: item %d title %q missing job name %q", i, row.Title(), want)
		}
	}
}

// TestS6EscTransitionsToS1 verifies Esc from S6 returns to S1.
func TestS6EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenPloyList {
		t.Errorf("Esc(S6): got screen %v, want ScreenPloyList", nm.screen)
	}
}

// TestS6ViewRendersSideBySide verifies that S6 view joins ploy and jobList.
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
	// Load runs first so job selection can resolve mig_id by run_id.
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{
			ID:        domaintypes.RunID("run-1"),
			MigID:     domaintypes.MigID("mig-1"),
			MigName:   "mig",
			CreatedAt: time.Now(),
		},
	}})
	m = next.(model)
	m.screen = ScreenJobsList
	next, _ = m.Update(jobsLoadedMsg{jobs: []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	nm := next.(model)
	nm.jobList = nm.jobList.Select(0)

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

	if item0.title != "mig" {
		t.Errorf("item 0 title: got %q, want %q", item0.title, "mig")
	}
	if item0.description != "mig-1" {
		t.Errorf("item 0 description: got %q, want %q", item0.description, "mig-1")
	}
	if item1.title != "Run" {
		t.Errorf("item 1 title: got %q, want %q", item1.title, "Run")
	}
	if item1.description != "run-1" {
		t.Errorf("item 1 description: got %q, want %q", item1.description, "run-1")
	}
	if item2.title != "Job" {
		t.Errorf("item 2 title: got %q, want %q", item2.title, "Job")
	}
	if item2.description != "job-1" {
		t.Errorf("item 2 description: got %q, want %q", item2.description, "job-1")
	}
}

func TestJobsStatusGlyphUsesColoredDotForTerminalStates(t *testing.T) {
	successGlyph := joblist.StatusGlyph(domaintypes.JobStatusSuccess)
	failGlyph := joblist.StatusGlyph(domaintypes.JobStatusFail)

	if !strings.Contains(successGlyph, "⏺") || !strings.Contains(successGlyph, "\x1b[") {
		t.Fatalf("success glyph should be a colored dot, got %q", successGlyph)
	}
	if !strings.Contains(failGlyph, "⏺") || !strings.Contains(failGlyph, "\x1b[") {
		t.Fatalf("failed glyph should be a colored dot, got %q", failGlyph)
	}
	if successGlyph == failGlyph {
		t.Fatalf("success and failed glyphs should use different colors, got success=%q fail=%q", successGlyph, failGlyph)
	}
}
