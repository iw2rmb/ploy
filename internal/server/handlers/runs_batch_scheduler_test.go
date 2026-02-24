package handlers

import (
	"context"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestBatchRepoStarter_StartPendingRepos_CreatesJobsWhenNone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.RunID("run_1")
	specID := domaintypes.SpecID("spec_1")
	repoID := domaintypes.MigRepoID("repo_1")

	st := &mockStore{
		getRunResult:  store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		listRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusQueued, RepoBaseRef: "main", Attempt: 1},
		},
		listQueuedRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusQueued, RepoBaseRef: "main", Attempt: 1},
		},
		listJobsByRunRepoAttemptResult: []store.Job{},
	}

	starter := NewBatchRepoStarter(st)
	got, err := starter.StartPendingRepos(ctx, runID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Pending != 1 {
		t.Fatalf("expected pending=1, got %d", got.Pending)
	}
	if got.Started != 1 {
		t.Fatalf("expected started=1, got %d", got.Started)
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs to be created, got %d", st.createJobCallCount)
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
	if st.scheduleNextJobCalled {
		t.Fatalf("expected ScheduleNextJob not to be called when creating jobs")
	}
}

func TestBatchRepoStarter_StartPendingRepos_SchedulesNextJobWhenNoActive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.RunID("run_1")
	specID := domaintypes.SpecID("spec_1")
	repoID := domaintypes.MigRepoID("repo_1")

	st := &mockStore{
		getRunResult:  store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		listRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusQueued, RepoBaseRef: "main", Attempt: 1},
		},
		listQueuedRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusQueued, RepoBaseRef: "main", Attempt: 1},
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{ID: domaintypes.JobID("job_1"), RunID: runID, RepoID: repoID, Attempt: 1, Status: store.JobStatusCreated},
			{ID: domaintypes.JobID("job_2"), RunID: runID, RepoID: repoID, Attempt: 1, Status: store.JobStatusCreated},
		},
	}

	starter := NewBatchRepoStarter(st)
	got, err := starter.StartPendingRepos(ctx, runID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Pending != 1 {
		t.Fatalf("expected pending=1, got %d", got.Pending)
	}
	if got.Started != 1 {
		t.Fatalf("expected started=1, got %d", got.Started)
	}
	if !st.scheduleNextJobCalled {
		t.Fatalf("expected ScheduleNextJob to be called")
	}
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs to be created, got %d", st.createJobCallCount)
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
}

func TestBatchRepoStarter_StartPendingRepos_SkipsTerminalRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.RunID("run_1")

	st := &mockStore{
		getRunResult: store.Run{ID: runID, SpecID: domaintypes.SpecID("spec_1"), Status: store.RunStatusFinished},
	}

	starter := NewBatchRepoStarter(st)
	got, err := starter.StartPendingRepos(ctx, runID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Started != 0 || got.Pending != 0 || got.AlreadyDone != 0 {
		t.Fatalf("expected zero result for terminal run, got %+v", got)
	}
	if st.listRunReposByRunCalled || st.listQueuedRunReposByRunCalled {
		t.Fatalf("expected no repo queries for terminal run")
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
}
