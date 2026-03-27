package joblist_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	clitui "github.com/iw2rmb/ploy/internal/client/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

// defaultItem is a local interface matching the title/description contract
// implemented by all list rows in this codebase.
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

func TestNewHasCorrectTitleAndWidth(t *testing.T) {
	m := joblist.New("JOBS")
	if m.Title() != "JOBS" {
		t.Errorf("Title: got %q, want %q", m.Title(), "JOBS")
	}
	if m.Width() != joblist.ListWidth {
		t.Errorf("Width: got %d, want %d", m.Width(), joblist.ListWidth)
	}
}

func TestSetJobsPopulatesItems(t *testing.T) {
	m := joblist.New("JOBS")
	nodeID := domaintypes.NodeID("abc123")
	m = m.SetJobs([]clitui.JobItem{
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
	})

	items := m.Items()
	if len(items) != 1 {
		t.Fatalf("Items: got %d, want 1", len(items))
	}
	row := mustDefaultItem(t, items[0])
	title := row.Title()
	desc := row.Description()

	if !strings.Contains(title, "⏺") || !strings.Contains(title, "deploy") {
		t.Errorf("title: expected status glyph and name, got %q", title)
	}
	if !strings.Contains(title, "\x1b[") {
		t.Errorf("title: expected ANSI color sequence for status glyph, got %q", title)
	}
	if !strings.HasSuffix(strings.TrimSpace(title), "2s") {
		t.Errorf("title: expected duration suffix %q, got %q", "2s", title)
	}
	if got := lipgloss.Width(title); got != joblist.ContentWidth {
		t.Errorf("title visible width: got %d, want %d", got, joblist.ContentWidth)
	}
	if desc != "job-abc" {
		t.Errorf("description: got %q, want %q", desc, "job-abc")
	}
}

func TestSetJobsPreservesOrder(t *testing.T) {
	m := joblist.New("JOBS")
	m = m.SetJobs([]clitui.JobItem{
		{Name: "first", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
		{Name: "second", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
		{Name: "third", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
	})

	items := m.Items()
	if len(items) != 3 {
		t.Fatalf("Items: got %d, want 3", len(items))
	}
	for i, want := range []string{"first", "second", "third"} {
		r := mustDefaultItem(t, items[i])
		if !strings.Contains(r.Title(), want) {
			t.Errorf("item %d title %q missing name %q", i, r.Title(), want)
		}
	}
}

func TestSelectedJobReturnsCurrentCursor(t *testing.T) {
	m := joblist.New("JOBS")
	jobs := []clitui.JobItem{
		{JobID: domaintypes.JobID("job-1"), Name: "first", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
		{JobID: domaintypes.JobID("job-2"), Name: "second", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
	}
	m = m.SetJobs(jobs)
	m = m.Select(1)

	job, ok := m.SelectedJob()
	if !ok {
		t.Fatal("SelectedJob: got false, want true")
	}
	if job.JobID != "job-2" {
		t.Errorf("SelectedJob: got %q, want %q", job.JobID, "job-2")
	}
}

func TestSelectedJobEmptyOnNoJobs(t *testing.T) {
	m := joblist.New("JOBS")
	_, ok := m.SelectedJob()
	if ok {
		t.Error("SelectedJob: got true on empty list, want false")
	}
}

func TestConfirmedJobIDDefaultsEmpty(t *testing.T) {
	m := joblist.New("JOBS")
	if id := m.ConfirmedJobID(); id != "" {
		t.Errorf("ConfirmedJobID on new model: got %q, want empty", id)
	}
}

func TestSetSelectedJobIDRoundTrip(t *testing.T) {
	m := joblist.New("JOBS")
	m = m.SetSelectedJobID(domaintypes.JobID("job-42"))
	if got := m.ConfirmedJobID(); got != "job-42" {
		t.Errorf("ConfirmedJobID: got %q, want %q", got, "job-42")
	}
}

func TestSetSelectedJobIDCanBeCleared(t *testing.T) {
	m := joblist.New("JOBS")
	m = m.SetSelectedJobID(domaintypes.JobID("job-1"))
	m = m.SetSelectedJobID("")
	if id := m.ConfirmedJobID(); id != "" {
		t.Errorf("ConfirmedJobID after clear: got %q, want empty", id)
	}
}

func TestDetailsNilByDefault(t *testing.T) {
	m := joblist.New("JOBS")
	if m.Details() != nil {
		t.Error("Details on new model: expected nil")
	}
}

func TestSetDetailsRoundTrip(t *testing.T) {
	m := joblist.New("JOBS")
	item := &cliruns.RepoJobEntry{JobID: domaintypes.JobID("job-99"), Name: "deploy"}
	m = m.SetDetails(item)
	got := m.Details()
	if got == nil {
		t.Fatal("Details: got nil, want non-nil")
	}
	if got.JobID != "job-99" {
		t.Errorf("Details.JobID: got %q, want %q", got.JobID, "job-99")
	}
}

func TestSetJobsClearsDetails(t *testing.T) {
	m := joblist.New("JOBS")
	m = m.SetDetails(&cliruns.RepoJobEntry{JobID: domaintypes.JobID("job-1")})
	m = m.SetJobs([]clitui.JobItem{
		{Name: "other", MigName: "m", RunID: domaintypes.RunID("r"), RepoID: domaintypes.RepoID("repo")},
	})
	if m.Details() != nil {
		t.Error("SetJobs: expected Details() to be cleared, got non-nil")
	}
}

func TestStatusGlyphTerminalStates(t *testing.T) {
	successGlyph := joblist.StatusGlyph(domaintypes.JobStatusSuccess)
	failGlyph := joblist.StatusGlyph(domaintypes.JobStatusFail)

	if !strings.Contains(successGlyph, "⏺") || !strings.Contains(successGlyph, "\x1b[") {
		t.Fatalf("success glyph should be a colored dot, got %q", successGlyph)
	}
	if !strings.Contains(failGlyph, "⏺") || !strings.Contains(failGlyph, "\x1b[") {
		t.Fatalf("fail glyph should be a colored dot, got %q", failGlyph)
	}
	if successGlyph == failGlyph {
		t.Fatalf("success and fail glyphs should differ, got success=%q fail=%q", successGlyph, failGlyph)
	}
}
