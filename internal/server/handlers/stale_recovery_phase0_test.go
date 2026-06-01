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
	waveID := domaintypes.NewWaveID()

	st := &jobStore{}
	st.cancelActiveJobsByRunRepoAttempt.val = 2
	st.listStaleRunningJobs.val = []store.ListStaleRunningJobsRow{
		{RunID: runID, Attempt: 1, RunningJobs: 2},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:          domaintypes.NewJobID(),
			RunID:       runID,
			Attempt:     1,
			Status:      domaintypes.JobStatusCancelled,
			JobType:     domaintypes.JobTypeMig,
			Meta:        []byte(`{"next_id":1000}`),
			NextID:      nil,
			Name:        "mig-0",
			RepoBaseRef: "main",
		},
	}
	st.countRunReposByStatus.val = []store.CountRunsByWaveStatusRow{
		{Status: domaintypes.RunStatusCancelled, Count: 1},
	}
	st.getRun.val = store.Run{ID: runID, WaveID: waveID, Status: domaintypes.RunStatusRunning}
	st.getWave.val = store.Wave{ID: waveID, Status: domaintypes.WaveStatusStarted}

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
		t.Fatal("expected CancelActiveJobsByRunAttempt to be called")
	}
	if !st.updateRunStatus.called {
		t.Fatal("expected UpdateRunStatus to be called during stale recovery")
	}
	if !st.updateRunStatus.called {
		t.Fatal("expected UpdateRunStatus to be called when all repos are terminal")
	}
	if len(st.updateRunStatus.calls) == 0 {
		t.Fatal("expected at least one UpdateRunStatus call")
	}
	if got := st.updateRunStatus.calls[len(st.updateRunStatus.calls)-1].Status; got != domaintypes.RunStatusCancelled {
		t.Fatalf("run repo status=%q, want %q", got, domaintypes.RunStatusCancelled)
	}
	if !st.updateWaveStatus.called {
		t.Fatal("expected UpdateWaveStatus to be called when all runs are terminal")
	}
	if st.updateWaveStatus.params.Status != domaintypes.WaveStatusFinished {
		t.Fatalf("wave status=%q, want %q", st.updateWaveStatus.params.Status, domaintypes.WaveStatusFinished)
	}
}

func TestStaleRecovery_RunCompletionNotTriggeredWhenOtherReposNonTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	waveID := domaintypes.NewWaveID()

	st := &jobStore{}
	st.cancelActiveJobsByRunRepoAttempt.val = 1
	st.listStaleRunningJobs.val = []store.ListStaleRunningJobsRow{
		{RunID: runID, Attempt: 1, RunningJobs: 1},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:          domaintypes.NewJobID(),
			RunID:       runID,
			Attempt:     1,
			Status:      domaintypes.JobStatusCancelled,
			JobType:     domaintypes.JobTypeMig,
			Meta:        []byte(`{"next_id":1000}`),
			NextID:      nil,
			Name:        "mig-0",
			RepoBaseRef: "main",
		},
	}
	st.countRunReposByStatus.val = []store.CountRunsByWaveStatusRow{
		{Status: domaintypes.RunStatusCancelled, Count: 1},
		{Status: domaintypes.RunStatusRunning, Count: 1},
	}
	st.getRun.val = store.Run{ID: runID, WaveID: waveID, Status: domaintypes.RunStatusRunning}
	st.getWave.val = store.Wave{ID: waveID, Status: domaintypes.WaveStatusStarted}

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

	if !st.updateRunStatus.called {
		t.Fatal("expected stale repo attempt status update")
	}
	if st.updateWaveStatus.called {
		t.Fatal("did not expect wave completion while another run is non-terminal")
	}
}
