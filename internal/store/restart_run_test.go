package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRestartRun_RequeuesTerminalRunAndRevivesFinishedWave(t *testing.T) {
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

	restarted, err := db.RestartRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("RestartRun() failed: %v", err)
	}
	if restarted.Status != types.RunStatusQueued {
		t.Fatalf("run status=%q, want %q", restarted.Status, types.RunStatusQueued)
	}
	if restarted.Attempt != fx.Run.Attempt+1 {
		t.Fatalf("run attempt=%d, want %d", restarted.Attempt, fx.Run.Attempt+1)
	}
	if restarted.LastError != nil {
		t.Fatalf("last_error=%q, want nil", *restarted.LastError)
	}
	if restarted.StartedAt.Valid || restarted.FinishedAt.Valid {
		t.Fatalf("run timing not cleared: started=%v finished=%v", restarted.StartedAt.Valid, restarted.FinishedAt.Valid)
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

			_, err := db.RestartRun(ctx, fx.Run.ID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RestartRun() error=%v, want %v", err, tt.wantErr)
			}
		})
	}
}

func strPtrForRestartRunTest(v string) *string {
	return &v
}
