package runs

import (
	"testing"

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

