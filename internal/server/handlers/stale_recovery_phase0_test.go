package handlers

import (
	"context"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestStaleRecovery_RepoStatusCancelledAndRunCompletionFinished(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &jobStore{}
	st.cancelActiveJobsByRunRepoAttempt.val = 2
	st.listStaleRunningJobs.val = []store.ListStaleRunningJobsRow{
		{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 2},
		}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:          domaintypes.NewJobID(),
			RunID:       runID,
			RepoID:      repoID,
			Attempt:     1,
			Status:      domaintypes.JobStatusCancelled,
			JobType:     domaintypes.JobTypeMod,
			Meta:        []byte(`{"next_id":1000}`),
			NextID:      nil,
			Name:        "mig-0",
			RepoBaseRef: "main",
		},
		}
	st.countRunReposByStatus.val = []store.CountRunReposByStatusRow{
		{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
		}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}

	task, err := recovery.NewStaleJobRecoveryTask(recovery.Options{
		Store:          st,
		Interval:       10 * time.Millisecond,
		NodeStaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !st.listStaleRunningJobs.called {
		t.Fatal("expected ListStaleRunningJobs to be called")
	}
	if !st.cancelActiveJobsByRunRepoAttempt.called {
		t.Fatal("expected CancelActiveJobsByRunRepoAttempt to be called")
	}
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called during stale recovery")
	}
	if !st.updateRunStatus.called {
		t.Fatal("expected UpdateRunStatus to be called when all repos are terminal")
	}
	if len(st.updateRunRepoStatus.calls) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	if got := st.updateRunRepoStatus.calls[len(st.updateRunRepoStatus.calls)-1].Status; got != domaintypes.RunRepoStatusCancelled {
		t.Fatalf("run repo status=%q, want %q", got, domaintypes.RunRepoStatusCancelled)
	}
	if st.updateRunStatus.params.Status != domaintypes.RunStatusFinished {
		t.Fatalf("run status=%q, want %q", st.updateRunStatus.params.Status, domaintypes.RunStatusFinished)
	}
}

func TestStaleRecovery_RunCompletionNotTriggeredWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &jobStore{}
	st.cancelActiveJobsByRunRepoAttempt.val = 1
	st.listStaleRunningJobs.val = []store.ListStaleRunningJobsRow{
		{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 1},
		}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:          domaintypes.NewJobID(),
			RunID:       runID,
			RepoID:      repoID,
			Attempt:     1,
			Status:      domaintypes.JobStatusCancelled,
			JobType:     domaintypes.JobTypeMod,
			Meta:        []byte(`{"next_id":1000}`),
			NextID:      nil,
			Name:        "mig-0",
			RepoBaseRef: "main",
		},
		}
	st.countRunReposByStatus.val = []store.CountRunReposByStatusRow{
		{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
		{Status: domaintypes.RunRepoStatusRunning, Count: 1},
		}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}

	task, err := recovery.NewStaleJobRecoveryTask(recovery.Options{
		Store:          st,
		Interval:       10 * time.Millisecond,
		NodeStaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewStaleJobRecoveryTask() error = %v", err)
	}

	if err := task.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !st.updateRunRepoStatus.called {
		t.Fatal("expected stale repo attempt status update")
	}
	if st.updateRunStatus.called {
		t.Fatal("did not expect run completion while another repo is non-terminal")
	}
}
