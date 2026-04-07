package runs

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestDeriveRunStateFromReport_IgnoresHistoricalJobFailureWhenRepoSucceeded(t *testing.T) {
	t.Parallel()

	report := RunReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunRepoStatusSuccess,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusFail},
					{Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}

	if got := DeriveRunStateFromReport(report); got != migsapi.RunStateSucceeded {
		t.Fatalf("DeriveRunStateFromReport() = %q, want %q", got, migsapi.RunStateSucceeded)
	}
}

func TestDeriveRunStateFromReport_RunningJobKeepsRunNonTerminal(t *testing.T) {
	t.Parallel()

	report := RunReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunRepoStatusCancelled,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusRunning},
				},
			},
		},
	}

	if got := DeriveRunStateFromReport(report); got != "" {
		t.Fatalf("DeriveRunStateFromReport() = %q, want empty", got)
	}
}

func TestDeriveRunStateFromReport_MixedSuccessAndCancelledIsCancelled(t *testing.T) {
	t.Parallel()

	report := RunReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunRepoStatusSuccess,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusSuccess},
				},
			},
			{
				Status: domaintypes.RunRepoStatusCancelled,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusCancelled},
				},
			},
		},
	}

	if got := DeriveRunStateFromReport(report); got != migsapi.RunStateCancelled {
		t.Fatalf("DeriveRunStateFromReport() = %q, want %q", got, migsapi.RunStateCancelled)
	}
}

func TestFollowModelKeyTogglesStdoutStderr(t *testing.T) {
	t.Parallel()

	model := newFollowModel(TextRenderOptions{}, true)
	next, _ := model.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	toggled := next.(followModel)
	if !toggled.expandStdout {
		t.Fatalf("expandStdout = false, want true after 'o'")
	}

	next, _ = toggled.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	toggled = next.(followModel)
	if !toggled.expandStderr {
		t.Fatalf("expandStderr = false, want true after 'e'")
	}
}

func TestFollowModelAppliesAndClearsPreviewRows(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	model := newFollowModel(TextRenderOptions{}, true)
	next, _ := model.Update(followJobPreviewMsg{
		jobID: jobID,
		preview: RunJobIOPreview{
			Stdout: []string{"line"},
			Stderr: []string{"err"},
		},
	})
	updated := next.(followModel)
	if len(updated.jobIOPreviews) != 1 {
		t.Fatalf("jobIOPreviews size = %d, want 1", len(updated.jobIOPreviews))
	}

	next, _ = updated.Update(followJobPreviewMsg{jobID: jobID, clearRow: true})
	cleared := next.(followModel)
	if len(cleared.jobIOPreviews) != 0 {
		t.Fatalf("jobIOPreviews size = %d, want 0", len(cleared.jobIOPreviews))
	}
}

func TestShouldTrackJobPreview(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "running", status: "running", want: true},
		{name: "started", status: "started", want: true},
		{name: "failed", status: "failed", want: false},
		{name: "error", status: "error", want: false},
		{name: "success", status: "success", want: false},
		{name: "cancelled", status: "cancelled", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldTrackJobPreview(tc.status); got != tc.want {
				t.Fatalf("shouldTrackJobPreview(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestFollowModelViewFiltersToRunningRepos(t *testing.T) {
	t.Parallel()

	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "follow-filter",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewMigRepoID(),
				RepoURL: "https://github.com/acme/running.git",
				Status:  domaintypes.RunRepoStatusRunning,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusRunning},
				},
			},
			{
				RepoID:  domaintypes.NewMigRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				Status:  domaintypes.RunRepoStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}
	model := newFollowModel(TextRenderOptions{}, true)
	model.report = &report

	view := strings.TrimSpace(model.View().Content)
	if !strings.Contains(view, "github.com/acme/running") {
		t.Fatalf("expected running repo in view, got %q", view)
	}
	if strings.Contains(view, "github.com/acme/done") {
		t.Fatalf("expected terminal repo to be hidden, got %q", view)
	}
}

func TestFollowModelViewShowsNoRunningReposMessage(t *testing.T) {
	t.Parallel()

	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "follow-empty",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewMigRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				Status:  domaintypes.RunRepoStatusFail,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusFail},
				},
			},
		},
	}
	model := newFollowModel(TextRenderOptions{}, true)
	model.report = &report

	view := strings.TrimSpace(model.View().Content)
	if !strings.Contains(view, "No repos with in-progress jobs.") {
		t.Fatalf("expected empty-running message in view, got %q", view)
	}
}

func TestWriteFinalStatusSnapshot_NonTTYUsesStatusRenderer(t *testing.T) {
	t.Parallel()

	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "final-snapshot",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewMigRepoID(),
				RepoURL: "https://github.com/acme/running.git",
				BaseRef: "main",
				Status:  domaintypes.RunRepoStatusRunning,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusRunning},
				},
			},
			{
				RepoID:  domaintypes.NewMigRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				BaseRef: "main",
				Status:  domaintypes.RunRepoStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}

	var out bytes.Buffer
	opts := TextRenderOptions{
		FilterRunningRepos: true,
		EmptyReposLine:     "No repos with in-progress jobs.",
		LiveDurations:      true,
		SpinnerFrame:       3,
	}
	if err := writeFinalStatusSnapshot(&out, report, opts); err != nil {
		t.Fatalf("writeFinalStatusSnapshot() error: %v", err)
	}

	rendered := out.String()
	if strings.Contains(rendered, "\x1b[2J\x1b[H") {
		t.Fatalf("expected no clear sequence for non-tty output, got %q", rendered)
	}
	if !strings.Contains(rendered, "   Repos: 2") {
		t.Fatalf("expected status snapshot with all repos, got %q", rendered)
	}
	if !strings.Contains(rendered, "github.com/acme/done") {
		t.Fatalf("expected terminal repo in final status snapshot, got %q", rendered)
	}
	if strings.Contains(rendered, "No repos with in-progress jobs.") {
		t.Fatalf("expected status snapshot semantics, got %q", rendered)
	}
}
