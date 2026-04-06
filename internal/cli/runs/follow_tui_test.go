package runs

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestDeriveRunStateFromReport_PrefersJobErrorOverCancelledRepos(t *testing.T) {
	t.Parallel()

	report := RunReport{
		Repos: []RunEntry{
			{
				Status: domaintypes.RunRepoStatusCancelled,
				Jobs: []RunJobEntry{
					{Status: domaintypes.JobStatusError},
					{Status: domaintypes.JobStatusCancelled},
				},
			},
		},
	}

	if got := DeriveRunStateFromReport(report); got != migsapi.RunStateError {
		t.Fatalf("DeriveRunStateFromReport() = %q, want %q", got, migsapi.RunStateError)
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
