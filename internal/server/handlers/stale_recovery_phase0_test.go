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

	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 2},
		},
		cancelActiveJobsByRunRepoAttemptResult: 2,
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				Attempt:     1,
				Status:      store.JobStatusCancelled,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{"next_id":1000}`),
				NextID:      nil,
				Name:        "mig-0",
				RepoBaseRef: "main",
			},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusCancelled, Count: 1},
		},
		getRunResult: store.Run{ID: runID, Status: store.RunStatusStarted},
	}

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

	if !st.listStaleRunningJobsCalled {
		t.Fatal("expected ListStaleRunningJobs to be called")
	}
	if !st.cancelActiveJobsByRunRepoAttemptCalled {
		t.Fatal("expected CancelActiveJobsByRunRepoAttempt to be called")
	}
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called during stale recovery")
	}
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called when all repos are terminal")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	if got := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1].Status; got != store.RunRepoStatusCancelled {
		t.Fatalf("run repo status=%q, want %q", got, store.RunRepoStatusCancelled)
	}
	if st.updateRunStatusParams.Status != store.RunStatusFinished {
		t.Fatalf("run status=%q, want %q", st.updateRunStatusParams.Status, store.RunStatusFinished)
	}
}

func TestStaleRecovery_RunCompletionNotTriggeredWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &mockStore{
		listStaleRunningJobsResult: []store.ListStaleRunningJobsRow{
			{RunID: runID, RepoID: repoID, Attempt: 1, RunningJobs: 1},
		},
		cancelActiveJobsByRunRepoAttemptResult: 1,
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				Attempt:     1,
				Status:      store.JobStatusCancelled,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{"next_id":1000}`),
				NextID:      nil,
				Name:        "mig-0",
				RepoBaseRef: "main",
			},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusCancelled, Count: 1},
			{Status: store.RunRepoStatusRunning, Count: 1},
		},
		getRunResult: store.Run{ID: runID, Status: store.RunStatusStarted},
	}

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

	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected stale repo attempt status update")
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect run completion while another repo is non-terminal")
	}
}
