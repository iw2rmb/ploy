package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

// Retention expires at the fixed deadline and never expires unknown run state.
func TestRunRetentionExpired(t *testing.T) {
	finished := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		status types.RunStatus
		age    time.Duration
		valid  bool
		want   bool
	}{
		{"success before deadline", types.RunStatusSuccess, 6*time.Hour - time.Nanosecond, true, false},
		{"success at deadline", types.RunStatusSuccess, 6 * time.Hour, true, true},
		{"failure before deadline", types.RunStatusFail, 24*time.Hour - time.Nanosecond, true, false},
		{"failure at deadline", types.RunStatusFail, 24 * time.Hour, true, true},
		{"cancelled at deadline", types.RunStatusCancelled, 24 * time.Hour, true, true},
		{"running", types.RunStatusRunning, 48 * time.Hour, true, false},
		{"unknown", types.RunStatus("unknown"), 48 * time.Hour, true, false},
		{"missing completion", types.RunStatusSuccess, 48 * time.Hour, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := Run{Status: tc.status, FinishedAt: pgtype.Timestamptz{Time: finished, Valid: tc.valid}}
			if got := runRetentionExpired(run, finished.Add(tc.age)); got != tc.want {
				t.Fatalf("expired=%v, want %v", got, tc.want)
			}
		})
	}
}

// Only expired runs with a nonempty terminal current attempt permit cleanup.
func TestGetRunCleanupStateRequiresTerminalCurrentAttempt(t *testing.T) {
	ctx, db, fx, _, jobs, _ := resumeFixture(t, 1)
	if _, err := db.Pool().Exec(ctx, `UPDATE runs SET finished_at=clock_timestamp()-interval '25 hours', stats='{"resume_count":2}' WHERE id=$1`, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	state, err := db.GetRunCleanupState(ctx, fx.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.CleanupEligible || state.Attempt != 1 || types.RunStats(state.Stats).ResumeCount() != 2 {
		t.Fatalf("cleanup state: %+v", state)
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE jobs SET status='Running' WHERE id=$1`, jobs[1].ID); err != nil {
		t.Fatal(err)
	}
	state, err = db.GetRunCleanupState(ctx, fx.Run.ID)
	if err != nil || state.CleanupEligible {
		t.Fatalf("active job: state=%+v err=%v", state, err)
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE runs SET attempt=2 WHERE id=$1`, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	state, err = db.GetRunCleanupState(ctx, fx.Run.ID)
	if err != nil || state.CleanupEligible {
		t.Fatalf("empty current attempt: state=%+v err=%v", state, err)
	}
}

// A pending resume must finish before eligibility can observe its run identity.
func TestGetRunCleanupStateWaitsForResumeAcrossExpiry(t *testing.T) {
	for _, commit := range []bool{true, false} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, db, fx, plans, _, _ := resumeFixture(t, 1)
			// The eligibility transaction begins before expiry but must use the clock after locking.
			if _, err := db.Pool().Exec(ctx, `UPDATE runs SET finished_at=clock_timestamp()-interval '24 hours'+interval '300 milliseconds' WHERE id=$1`, fx.Run.ID); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			q := db.(*PgStore).Queries.WithTx(tx)
			run, err := q.GetRunForUpdate(ctx, fx.Run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := resumeFailedJobs(ctx, q, run, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, Jobs: plans}); err != nil {
				t.Fatal(err)
			}
			type result struct {
				state RunCleanupState
				err   error
			}
			done := make(chan result, 1)
			go func() { state, err := db.GetRunCleanupState(ctx, fx.Run.ID); done <- result{state, err} }()
			select {
			case result := <-done:
				t.Fatalf("read passed uncommitted resume: %+v", result)
			case <-time.After(400 * time.Millisecond):
			}
			if commit {
				err = tx.Commit(ctx)
			} else {
				err = tx.Rollback(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := <-done
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.state.CleanupEligible == commit {
				t.Fatalf("commit=%v cleanup state=%+v", commit, got.state)
			}
			if commit && (got.state.Status != types.RunStatusRunning || types.RunStats(got.state.Stats).ResumeCount() != 1) {
				t.Fatalf("stale identity: %+v", got.state)
			}
		})
	}
}

// Expired retained resume preserves jobs and logs while full restart remains available.
func TestRestartRunExpiredResumePreservesDataAndAllowsFullRestart(t *testing.T) {
	ctx, db, fx, plans, jobs, _ := resumeFixture(t, 1)
	if _, err := db.Pool().Exec(ctx, `UPDATE runs SET finished_at=clock_timestamp()-interval '24 hours' WHERE id=$1`, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateLog(ctx, CreateLogParams{RunID: fx.Run.ID, JobID: &jobs[1].ID, ChunkNo: 0, DataSize: 1}); err != nil {
		t.Fatal(err)
	}
	_, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans})
	if !errors.Is(err, ErrRunResumeExpired) {
		t.Fatalf("resume error=%v", err)
	}
	after, err := db.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: fx.Run.ID, Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[types.JobID]Job, len(jobs))
	for _, job := range jobs {
		before[job.ID] = job
	}
	for _, job := range after {
		if !reflect.DeepEqual(job, before[job.ID]) {
			t.Fatalf("expired resume changed job %s", job.ID)
		}
	}
	var logs int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM logs WHERE job_id=$1`, jobs[1].ID).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("expired resume removed logs: %d", logs)
	}
	run, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, Jobs: plans})
	if err != nil {
		t.Fatal(err)
	}
	if run.Attempt != 2 || run.Status != types.RunStatusRunning {
		t.Fatalf("full restart: %+v", run)
	}
}

// Resume checks the current clock after waiting for the run lock across expiry.
func TestRestartRunResumeExpiresWhileWaitingForRunLock(t *testing.T) {
	ctx, db, fx, plans, _, _ := resumeFixture(t, 1)
	if _, err := db.Pool().Exec(ctx, `UPDATE runs SET finished_at=clock_timestamp()-interval '24 hours'+interval '300 milliseconds' WHERE id=$1`, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := db.(*PgStore).Queries.WithTx(tx).GetRunForUpdate(ctx, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("resume passed the run lock: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrRunResumeExpired) {
		t.Fatalf("resume after expiry: %v", err)
	}
}
