package store

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
)

func TestRestartRun_MaterializesNextAttemptAndRevivesFinishedWave(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/restart-run", "main", []byte(`{"type":"restart-run"}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusSuccess}); err != nil {
		t.Fatalf("UpdateRunStatus(success) failed: %v", err)
	}
	if err := db.UpdateRunError(ctx, UpdateRunErrorParams{ID: fx.Run.ID, LastError: strPtrForRestartRunTest("previous failure")}); err != nil {
		t.Fatalf("UpdateRunError() failed: %v", err)
	}
	if err := db.UpdateWaveStatus(ctx, UpdateWaveStatusParams{ID: fx.Wave.ID, Status: types.WaveStatusFinished}); err != nil {
		t.Fatalf("UpdateWaveStatus(finished) failed: %v", err)
	}
	activeJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.Run.RepoID, fx.Run.RepoBaseRef, fx.Run.Attempt, "active", types.JobStatusCreated)

	restarted, err := db.RestartRun(ctx, RestartRunParams{
		RunID:           fx.Run.ID,
		ExpectedAttempt: fx.Run.Attempt,
		Jobs:            plannedJobsForStoreTest("new-head", "new-tail"),
	})
	if err != nil {
		t.Fatalf("RestartRun() failed: %v", err)
	}
	if restarted.Status != types.RunStatusRunning {
		t.Fatalf("run status=%q, want %q", restarted.Status, types.RunStatusRunning)
	}
	if restarted.Attempt != fx.Run.Attempt+1 {
		t.Fatalf("run attempt=%d, want %d", restarted.Attempt, fx.Run.Attempt+1)
	}
	if restarted.LastError != nil {
		t.Fatalf("last_error=%q, want nil", *restarted.LastError)
	}
	if !restarted.StartedAt.Valid || restarted.FinishedAt.Valid {
		t.Fatalf("run timing not reset for execution: started=%v finished=%v", restarted.StartedAt.Valid, restarted.FinishedAt.Valid)
	}
	if string(restarted.Stats) != "{}" {
		t.Fatalf("run stats=%s, want {}", string(restarted.Stats))
	}

	wave, err := db.GetWave(ctx, fx.Wave.ID)
	if err != nil {
		t.Fatalf("GetWave() failed: %v", err)
	}
	if wave.Status != types.WaveStatusStarted {
		t.Fatalf("wave status=%q, want %q", wave.Status, types.WaveStatusStarted)
	}
	if wave.FinishedAt.Valid {
		t.Fatal("wave finished_at must be cleared when restarted")
	}

	job, err := db.GetJob(ctx, activeJob.ID)
	if err != nil {
		t.Fatalf("GetJob() failed: %v", err)
	}
	if job.Status != types.JobStatusCancelled {
		t.Fatalf("active job status=%q, want %q", job.Status, types.JobStatusCancelled)
	}
	newJobs, err := db.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: fx.Run.ID, Attempt: restarted.Attempt})
	if err != nil || len(newJobs) != 2 {
		t.Fatalf("new attempt jobs=%d err=%v, want 2 jobs", len(newJobs), err)
	}
}

func TestRestartRun_RejectsActiveRunsAndCancelledWaves(t *testing.T) {
	ctx, db := newTestStore(t)

	tests := []struct {
		name    string
		setup   func(v1Fixture)
		wantErr error
	}{
		{
			name: "active run",
			setup: func(fx v1Fixture) {
				if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusRunning}); err != nil {
					t.Fatalf("UpdateRunStatus(running) failed: %v", err)
				}
			},
			wantErr: ErrRunRestartActive,
		},
		{
			name: "cancelled wave",
			setup: func(fx v1Fixture) {
				if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusFail}); err != nil {
					t.Fatalf("UpdateRunStatus(fail) failed: %v", err)
				}
				if err := db.UpdateWaveStatus(ctx, UpdateWaveStatusParams{ID: fx.Wave.ID, Status: types.WaveStatusCancelled}); err != nil {
					t.Fatalf("UpdateWaveStatus(cancelled) failed: %v", err)
				}
			},
			wantErr: ErrRunRestartWaveCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newV1Fixture(t, ctx, db, "https://github.com/test/restart-"+strings.ReplaceAll(tt.name, " ", "-"), "main", []byte(`{"type":"restart-run-reject"}`))
			tt.setup(fx)

			_, err := db.RestartRun(ctx, RestartRunParams{
				RunID:           fx.Run.ID,
				ExpectedAttempt: fx.Run.Attempt,
				Jobs:            plannedJobsForStoreTest(),
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RestartRun() error=%v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRestartRun_WritesStatsForNewAttempt(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/restart-run-stats", "main", []byte(`{"type":"restart-run-stats"}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusFail}); err != nil {
		t.Fatalf("UpdateRunStatus(fail) failed: %v", err)
	}
	hash := gitlabtoken.Hash("glpat-restart-secret")
	stats, err := gitlabtoken.RunStatsWithMarker(hash)
	if err != nil {
		t.Fatalf("RunStatsWithMarker() failed: %v", err)
	}

	restarted, err := db.RestartRun(ctx, RestartRunParams{
		RunID:           fx.Run.ID,
		ExpectedAttempt: fx.Run.Attempt,
		Stats:           stats,
		Jobs:            plannedJobsForStoreTest(),
	})
	if err != nil {
		t.Fatalf("RestartRun() failed: %v", err)
	}
	if got := gitlabtoken.HashFromRunStats(restarted.Stats); got != hash {
		t.Fatalf("token hash marker=%q, want %q", got, hash)
	}
}

func TestRestartRun_ConcurrentCallsCreateOneAttempt(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/restart-run-concurrent", "main", []byte(`{"steps":[{"image":"a"}]}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusFail}); err != nil {
		t.Fatalf("UpdateRunStatus(fail) failed: %v", err)
	}

	plans := [][]PlannedJob{
		plannedJobsForStoreTest("first-head", "first-tail"),
		plannedJobsForStoreTest("second-head", "second-tail"),
	}
	results := make(chan error, len(plans))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, jobs := range plans {
		jobs := jobs
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := db.RestartRun(ctx, RestartRunParams{
				RunID:           fx.Run.ID,
				ExpectedAttempt: fx.Run.Attempt,
				Jobs:            jobs,
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRunRestartActive):
			conflicts++
		default:
			t.Fatalf("RestartRun() unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("restart results: successes=%d conflicts=%d, want 1 each", successes, conflicts)
	}

	run, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if run.Attempt != fx.Run.Attempt+1 || run.Status != types.RunStatusRunning {
		t.Fatalf("run attempt=%d status=%s, want attempt=%d status=Running", run.Attempt, run.Status, fx.Run.Attempt+1)
	}
	jobs, err := db.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: run.ID, Attempt: run.Attempt})
	if err != nil {
		t.Fatalf("ListJobsByRunAttempt() failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("new attempt jobs=%d, want one 2-job chain", len(jobs))
	}
}

func strPtrForRestartRunTest(v string) *string {
	return &v
}
