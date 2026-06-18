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

	report := RunStatusReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunStatusSuccess,
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

	report := RunStatusReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunStatusCancelled,
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

	report := RunStatusReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusSuccess},
				},
			},
			{
				Status: domaintypes.RunStatusCancelled,
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

func TestFollowModelFinalViewUsesStatusSnapshotSemantics(t *testing.T) {
	t.Parallel()

	firstJobID := domaintypes.NewJobID()
	secondJobID := domaintypes.NewJobID()
	report := RunStatusReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "final-snapshot",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/one.git",
				BaseRef: "main",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: firstJobID, JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/two.git",
				BaseRef: "main",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: secondJobID, JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}

	model := newFollowModel(TextRenderOptions{}, true)
	next, _ := model.Update(followTerminalMsg{report: report, state: migsapi.RunStateSucceeded})
	rendered := next.(followModel).View().Content
	if !strings.Contains(rendered, firstJobID.String()) || !strings.Contains(rendered, secondJobID.String()) {
		t.Fatalf("expected final view to include all job rows, got %q", rendered)
	}
	if strings.Contains(rendered, "No repos with in-progress jobs.") {
		t.Fatalf("expected final view to avoid in-progress-only empty text, got %q", rendered)
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

	report := RunStatusReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "follow-filter",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/running.git",
				Status:  domaintypes.RunStatusRunning,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusRunning},
				},
			},
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}
	model := newFollowModel(TextRenderOptions{}, true)
	model.report = &report

	view := strings.TrimSpace(model.View().Content)
	if !strings.Contains(view, "acme/running") {
		t.Fatalf("expected running repo in view, got %q", view)
	}
	if strings.Contains(view, "acme/done") {
		t.Fatalf("expected terminal repo to be hidden, got %q", view)
	}
}

func TestFollowModelViewShowsNoRunningReposMessage(t *testing.T) {
	t.Parallel()

	report := RunStatusReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "follow-empty",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				Status:  domaintypes.RunStatusFail,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusFail},
				},
			},
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/success.git",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusSuccess},
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

func TestFollowModelViewShowsWaitingMessageForQueuedRuns(t *testing.T) {
	t.Parallel()

	report := RunStatusReport{
		RunID:       domaintypes.NewRunID(),
		MigID:       domaintypes.NewMigID(),
		MigName:     "follow-waiting",
		SpecID:      domaintypes.NewSpecID(),
		WaitingRuns: 3,
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/waiting-a.git",
				Status:  domaintypes.RunStatusQueued,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusQueued},
				},
			},
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/waiting-b.git",
				Status:  domaintypes.RunStatusRunning,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusCreated},
				},
			},
		},
	}
	model := newFollowModel(TextRenderOptions{}, true)
	model.report = &report

	view := strings.TrimSpace(model.View().Content)
	if !strings.Contains(view, "Waiting for 3 run(s) to finish.") {
		t.Fatalf("expected waiting message in view, got %q", view)
	}
	if strings.Contains(view, "No repos with in-progress jobs.") {
		t.Fatalf("did not expect empty-running message in waiting view, got %q", view)
	}
}

func TestFollowModelViewSingleRepoKeepsRepoVisibleWithoutRunningJobs(t *testing.T) {
	t.Parallel()

	report := RunStatusReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "follow-single-repo",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}
	model := newFollowModel(TextRenderOptions{}, true)
	model.report = &report

	view := strings.TrimSpace(model.View().Content)
	if !strings.Contains(view, "acme/done") {
		t.Fatalf("expected single repo to stay visible in view, got %q", view)
	}
	if strings.Contains(view, "No repos with in-progress jobs.") {
		t.Fatalf("expected single repo to render instead of empty-running message, got %q", view)
	}
}

func TestWriteFinalStatusSnapshot_UsesStatusRenderer(t *testing.T) {
	t.Parallel()

	doneJobID := domaintypes.NewJobID()
	report := RunStatusReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "final-snapshot",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/running.git",
				BaseRef: "main",
				Status:  domaintypes.RunStatusRunning,
				Jobs: []RunJobEntry{
					{JobID: domaintypes.NewJobID(), JobType: "mig", Status: domaintypes.JobStatusRunning},
				},
			},
			{
				RepoID:  domaintypes.NewRepoID(),
				RepoURL: "https://github.com/acme/done.git",
				BaseRef: "main",
				Status:  domaintypes.RunStatusSuccess,
				Jobs: []RunJobEntry{
					{JobID: doneJobID, JobType: "mig", Status: domaintypes.JobStatusSuccess},
				},
			},
		},
	}

	tests := []struct {
		name              string
		clearBeforeRender bool
		wantClearPrefix   bool
	}{
		{name: "without clear", clearBeforeRender: false, wantClearPrefix: false},
		{name: "with clear", clearBeforeRender: true, wantClearPrefix: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			opts := TextRenderOptions{
				FilterRunningRepos: true,
				EmptyReposLine:     "No repos with in-progress jobs.",
				LiveDurations:      true,
				SpinnerFrame:       3,
				JobIOPreviews: map[domaintypes.JobID]RunJobIOPreview{
					report.Repos[0].Jobs[0].JobID: {Stdout: []string{"preview"}},
				},
				ExpandStdout: true,
			}
			if err := writeFinalStatusSnapshot(&out, report, opts, tc.clearBeforeRender); err != nil {
				t.Fatalf("writeFinalStatusSnapshot() error: %v", err)
			}

			rendered := out.String()
			if got := strings.HasPrefix(rendered, clearScreenSequence); got != tc.wantClearPrefix {
				t.Fatalf("clear prefix = %v, want %v in %q", got, tc.wantClearPrefix, rendered)
			}
			if strings.Contains(rendered, "   Repos:") {
				t.Fatalf("did not expect repo count in final status snapshot, got %q", rendered)
			}
			if !strings.Contains(rendered, doneJobID.String()) {
				t.Fatalf("expected terminal job row in final status snapshot, got %q", rendered)
			}
			if strings.Contains(rendered, "No repos with in-progress jobs.") {
				t.Fatalf("expected status snapshot semantics, got %q", rendered)
			}
			if strings.Contains(rendered, "preview") {
				t.Fatalf("expected follow preview to be omitted from final status snapshot, got %q", rendered)
			}
		})
	}
}
