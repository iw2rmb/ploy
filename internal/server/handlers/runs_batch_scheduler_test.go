package handlers

import (
	"context"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const testRunRepoSHA0 = "0123456789abcdef0123456789abcdef01234567"

func TestBatchRepoStarter_StartPendingRepos_CreatesJobsWhenNone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	waveID := domaintypes.WaveID("2tWAVETest000000000000000")
	runID := domaintypes.RunID("2tRUNTest0000000000000000")
	specID := domaintypes.SpecID("spec_1")
	repoID := domaintypes.RepoID("repo_1")

	st := &runStore{}
	st.getWave.val = store.Wave{ID: waveID, SpecID: specID, Status: domaintypes.WaveStatusStarted}
	st.getRun.val = store.Run{ID: runID, SpecID: specID, Status: domaintypes.RunStatusRunning}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)}
	st.listRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}
	st.listQueuedRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{}

	starter := NewBatchRepoStarter(st, nil)
	got, err := starter.StartPendingRepos(ctx, waveID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Pending != 1 {
		t.Fatalf("expected pending=1, got %d", got.Pending)
	}
	if got.Started != 1 {
		t.Fatalf("expected started=1, got %d", got.Started)
	}
	if len(st.createJob.calls) != 3 {
		t.Fatalf("expected 3 jobs to be created, got %d", len(st.createJob.calls))
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
	if st.scheduleNextJob.called {
		t.Fatalf("expected ScheduleNextJob not to be called when creating jobs")
	}
}

func TestBatchRepoStarter_StartPendingRepos_InvalidStoredSpec(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	waveID := domaintypes.WaveID("2tWAVETest000000000000000")
	runID := domaintypes.RunID("2tRUNTest0000000000000000")
	specID := domaintypes.SpecID("spec_1")
	repoID := domaintypes.RepoID("repo_1")

	st := &runStore{}
	st.getWave.val = store.Wave{ID: waveID, SpecID: specID, Status: domaintypes.WaveStatusStarted}
	st.getRun.val = store.Run{ID: runID, SpecID: specID, Status: domaintypes.RunStatusRunning}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"version":"old","steps":[{"image":"a"}]}`)}
	st.listRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}
	st.listQueuedRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}

	starter := NewBatchRepoStarter(st, nil)
	if _, err := starter.StartPendingRepos(ctx, waveID); err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if !st.updateRunRepoError.called {
		t.Fatal("expected run repo error to be recorded")
	}
	if st.updateRunRepoError.params.LastError == nil || !strings.Contains(*st.updateRunRepoError.params.LastError, "parse migs spec") {
		t.Fatalf("last_error = %v, want parse migs spec", st.updateRunRepoError.params.LastError)
	}
	if len(st.createJob.calls) != 0 {
		t.Fatalf("expected no jobs to be created, got %d", len(st.createJob.calls))
	}
}

func TestBatchRepoStarter_StartPendingRepos_SchedulesNextJobWhenNoActive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	waveID := domaintypes.WaveID("2tWAVETest000000000000000")
	runID := domaintypes.RunID("2tRUNTest0000000000000000")
	specID := domaintypes.SpecID("spec_1")
	repoID := domaintypes.RepoID("repo_1")

	st := &runStore{}
	st.getWave.val = store.Wave{ID: waveID, SpecID: specID, Status: domaintypes.WaveStatusStarted}
	st.getRun.val = store.Run{ID: runID, SpecID: specID, Status: domaintypes.RunStatusRunning}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)}
	st.listRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}
	st.listQueuedRunReposByRun.val = []store.Run{
		{ID: runID, RepoID: repoID, Status: domaintypes.RunStatusQueued, RepoBaseRef: "main", RepoSha0: testRunRepoSHA0, Attempt: 1},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: domaintypes.JobID("job_1"), RunID: runID, RepoID: repoID, Attempt: 1, Status: domaintypes.JobStatusCreated},
		{ID: domaintypes.JobID("job_2"), RunID: runID, RepoID: repoID, Attempt: 1, Status: domaintypes.JobStatusCreated},
	}

	starter := NewBatchRepoStarter(st, nil)
	got, err := starter.StartPendingRepos(ctx, waveID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Pending != 1 {
		t.Fatalf("expected pending=1, got %d", got.Pending)
	}
	if got.Started != 1 {
		t.Fatalf("expected started=1, got %d", got.Started)
	}
	if !st.scheduleNextJob.called {
		t.Fatalf("expected ScheduleNextJob to be called")
	}
	if len(st.createJob.calls) != 0 {
		t.Fatalf("expected no jobs to be created, got %d", len(st.createJob.calls))
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
}

func TestBatchRepoStarter_StartPendingRepos_SkipsTerminalRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	waveID := domaintypes.WaveID("2tWAVETest000000000000000")

	st := &runStore{}
	st.getWave.val = store.Wave{ID: waveID, SpecID: domaintypes.SpecID("spec_1"), Status: domaintypes.WaveStatusFinished}

	starter := NewBatchRepoStarter(st, nil)
	got, err := starter.StartPendingRepos(ctx, waveID)
	if err != nil {
		t.Fatalf("StartPendingRepos returned error: %v", err)
	}
	if got.Started != 0 || got.Pending != 0 || got.AlreadyDone != 0 {
		t.Fatalf("expected zero result for terminal run, got %+v", got)
	}
	if st.listRunReposByRun.called || st.listQueuedRunReposByRun.called {
		t.Fatalf("expected no repo queries for terminal run")
	}
	if st.createRunCalled {
		t.Fatalf("expected CreateRun not to be called (no child runs per repo)")
	}
}
