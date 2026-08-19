package joblist_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

type defaultItem interface {
	Title() string
	Description() string
}

func mustDefaultItem(t *testing.T, value any) defaultItem {
	t.Helper()
	item, ok := value.(defaultItem)
	if !ok {
		t.Fatalf("item has type %T, want defaultItem", value)
	}
	return item
}

func TestNewHasCorrectTitleAndWidth(t *testing.T) {
	m := joblist.New("JOBS")
	if m.Title() != "JOBS" {
		t.Errorf("Title() = %q, want %q", m.Title(), "JOBS")
	}
	if m.Width() != joblist.ListWidth {
		t.Errorf("Width() = %d, want %d", m.Width(), joblist.ListWidth)
	}
}

func TestSetJobsFormatsRows(t *testing.T) {
	m := joblist.New("JOBS")
	nodeID := domaintypes.NodeID("abc123")
	m = m.SetJobs([]domainapi.JobListItem{{
		JobID: domaintypes.JobID("job-abc"), Name: "deploy", Status: domaintypes.JobStatusSuccess,
		DurationMs: 2500, JobImage: "ghcr.io/iw2rmb/ploy/migs-java17:latest", NodeID: &nodeID,
		MigName: "my-mig", RunID: domaintypes.RunID("run-xyz"), RepoID: domaintypes.RepoID("repo-123"),
	}})

	items := m.Items()
	if len(items) != 1 {
		t.Fatalf("Items() count = %d, want 1", len(items))
	}
	row := mustDefaultItem(t, items[0])
	title := row.Title()
	if !strings.Contains(title, "⏺") || !strings.Contains(title, "deploy") || !strings.Contains(title, "\x1b[") {
		t.Errorf("row title missing colored status glyph or name: %q", title)
	}
	if !strings.HasSuffix(title, " 2s") {
		t.Errorf("row title duration alignment = %q, want suffix %q", title, " 2s")
	}
	if got := lipgloss.Width(title); got != joblist.ContentWidth {
		t.Errorf("row title width = %d, want %d", got, joblist.ContentWidth)
	}
	if strings.Contains(title, "...") || strings.Contains(title, "…") {
		t.Errorf("row title must not use ellipsis: %q", title)
	}
	if got := row.Description(); got != "job-abc" {
		t.Errorf("row description = %q, want %q", got, "job-abc")
	}
}

func TestSetJobsPreservesOrder(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{
		{Name: "first"}, {Name: "second"}, {Name: "third"},
	})
	items := m.Items()
	if len(items) != 3 {
		t.Fatalf("Items() count = %d, want 3", len(items))
	}
	for i, want := range []string{"first", "second", "third"} {
		if title := mustDefaultItem(t, items[i]).Title(); !strings.Contains(title, want) {
			t.Errorf("item %d title %q missing name %q", i, title, want)
		}
	}
}

func TestHighlightedSelectionFollowsCursor(t *testing.T) {
	m := joblist.New("JOBS")
	if _, ok := m.SelectedJob(); ok || m.SelectedJobID() != "" {
		t.Fatal("empty model has a highlighted selection")
	}

	m = m.SetJobs([]domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), Name: "first"},
		{JobID: domaintypes.JobID("job-2"), Name: "second"},
	}).Select(1)
	job, ok := m.SelectedJob()
	if !ok || job.JobID != "job-2" || m.SelectedJobID() != "job-2" {
		t.Errorf("highlighted selection = (%+v, %v, %q), want job-2", job, ok, m.SelectedJobID())
	}
}

func TestConfirmedSelectionCanBeSetAndCleared(t *testing.T) {
	m := joblist.New("JOBS")
	if got := m.ConfirmedJobID(); got != "" {
		t.Fatalf("initial ConfirmedJobID() = %q, want empty", got)
	}
	m = m.SetSelectedJobID(domaintypes.JobID("job-42"))
	if got := m.ConfirmedJobID(); got != "job-42" {
		t.Errorf("confirmed selection after set = %q, want %q", got, "job-42")
	}
	m = m.SetSelectedJobID("")
	if got := m.ConfirmedJobID(); got != "" {
		t.Errorf("confirmed selection after clear = %q, want empty", got)
	}
}

func TestDetailsCanBeSetAndClearedByRefresh(t *testing.T) {
	m := joblist.New("JOBS")
	if m.Details() != nil {
		t.Fatal("initial Details() is non-nil")
	}
	m = m.SetDetails(&cliruns.RunJobDetailEntry{JobID: domaintypes.JobID("job-99"), Name: "deploy"})
	if got := m.Details(); got == nil || got.JobID != "job-99" {
		t.Fatalf("Details() after set = %+v, want job-99", got)
	}
	m = m.SetJobs([]domainapi.JobListItem{{JobID: domaintypes.JobID("job-100"), Name: "other"}})
	if m.Details() != nil {
		t.Error("SetJobs() did not clear details")
	}
}

func TestStatusGlyphTerminalStates(t *testing.T) {
	success := joblist.StatusGlyph(domaintypes.JobStatusSuccess)
	failure := joblist.StatusGlyph(domaintypes.JobStatusFail)
	if !strings.Contains(success, "⏺") || !strings.Contains(success, "\x1b[") {
		t.Fatalf("success glyph is not a colored dot: %q", success)
	}
	if !strings.Contains(failure, "⏺") || !strings.Contains(failure, "\x1b[") {
		t.Fatalf("failure glyph is not a colored dot: %q", failure)
	}
	if success == failure {
		t.Fatalf("success and failure glyphs are equal: %q", success)
	}
}
