package store

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

func resumeFixture(t *testing.T, failedIndex int) (context.Context, Store, v1Fixture, []JobPlan, []Job, Node) {
	t.Helper()
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://example.com/resume", "main", []byte(`{}`))
	plans := plannedJobsForStoreTest("pre-gate", "migration", "post-gate")
	jobs, err := createPlannedJobs(ctx, db.(*PgStore).Queries, fx.Run, plans)
	if err != nil {
		t.Fatal(err)
	}
	node := createTestNode(t, ctx, db)
	for i, job := range jobs {
		status := types.JobStatusSuccess
		if i == failedIndex {
			status = types.JobStatusError
		}
		if i > failedIndex {
			status = types.JobStatusCancelled
		}
		var owner any
		if i <= failedIndex {
			owner = node.ID
		}
		_, err := db.Pool().Exec(ctx, `UPDATE jobs SET status=$2, node_id=$3,
   started_at=CASE WHEN $3::text IS NOT NULL THEN now()-interval '1 minute' END,
   finished_at=now(), duration_ms=60000, exit_code=-1, repo_sha_in=$4,
   repo_sha_in8=substring($4,1,8) WHERE id=$1`, job.ID, status, owner, testSHA)
		if err != nil {
			t.Fatal(err)
		}
	}
	runStatus := types.RunStatusFail
	if failedIndex < len(jobs)-1 {
		runStatus = types.RunStatusCancelled
	}
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: runStatus}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateWaveStatus(ctx, UpdateWaveStatusParams{ID: fx.Wave.ID, Status: types.WaveStatusFinished}); err != nil {
		t.Fatal(err)
	}
	for i := range jobs {
		jobs[i], err = db.GetJob(ctx, jobs[i].ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return ctx, db, fx, plans, jobs, node
}

// A retry keeps successful work, restores the suffix, and remains on the original node.
func TestRestartRun_FromFailedResumesLinkedSuffix(t *testing.T) {
	for _, failed := range []int{0, 1, 2} {
		t.Run([]string{"head", "middle", "tail"}[failed], func(t *testing.T) {
			ctx, db, fx, plans, jobs, node := resumeFixture(t, failed)
			_, err := db.CreateLog(ctx, CreateLogParams{RunID: fx.Run.ID, JobID: &jobs[failed].ID, ChunkNo: 0, DataSize: 1})
			if err != nil {
				t.Fatal(err)
			}
			run, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans})
			if err != nil {
				t.Fatal(err)
			}
			if run.Attempt != 1 || run.Status != types.RunStatusRunning || run.FinishedAt.Valid || types.RunStats(run.Stats).ResumeCount() != 1 || types.RunStats(run.Stats).LastResumedAt() == "" {
				t.Fatalf("invalid resumed run: %+v", run)
			}
			for i, old := range jobs {
				got, err := db.GetJob(ctx, old.ID)
				if err != nil {
					t.Fatal(err)
				}
				if i < failed {
					if !reflect.DeepEqual(got, old) {
						t.Fatalf("successful job changed: %s", old.ID)
					}
				} else {
					want := types.JobStatusCreated
					if i == failed {
						want = types.JobStatusQueued
					}
					if got.Status != want || got.StartedAt.Valid || got.FinishedAt.Valid || got.ExitCode != nil || got.DurationMs != 0 || got.RepoShaOut != "" {
						t.Fatalf("invalid reset job: %+v", got)
					}
				}
			}
			other := createTestNode(t, ctx, db)
			if _, err = db.ClaimJob(ctx, other.ID); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("foreign node claimed retry: %v", err)
			}
			claimed, err := db.ClaimJob(ctx, node.ID)
			if err != nil || claimed.ID != jobs[failed].ID {
				t.Fatalf("claim: %+v %v", claimed, err)
			}
			if err = db.UnclaimJob(ctx, UnclaimJobParams{ID: claimed.ID, NodeID: node.ID}); err != nil {
				t.Fatal(err)
			}
			if _, err = db.ClaimJob(ctx, other.ID); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("unclaim lost affinity: %v", err)
			}
			claimed, err = db.ClaimJob(ctx, node.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.CreateLog(ctx, CreateLogParams{RunID: fx.Run.ID, JobID: &claimed.ID, ChunkNo: 0, DataSize: 1}); err != nil {
				t.Fatalf("retry log collides: %v", err)
			}
			called := false
			err = db.WithJobExecution(ctx, claimed.ID, 0, func(Store, Job) error { called = true; return nil })
			if !errors.Is(err, ErrJobExecutionStale) || called {
				t.Fatalf("old completion accepted: %v", err)
			}
			err = db.WithJobExecution(ctx, claimed.ID, 1, func(st Store, j Job) error {
				return st.UpdateJobCompletion(ctx, UpdateJobCompletionParams{ID: j.ID, Status: types.JobStatusSuccess, RepoShaOut: testSHA})
			})
			if err != nil {
				t.Fatal(err)
			}
			if failed < 2 {
				if _, err = db.PromoteJobByIDIfUnblocked(ctx, jobs[failed+1].ID); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Concurrent retry requests schedule one execution and increment resume history once.
func TestRestartRun_FromFailedConcurrent(t *testing.T) {
	ctx, db, fx, plans, _, _ := resumeFixture(t, 1)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrRunRestartActive) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful retries=%d", successes)
	}
}

// A malformed or partly executed suffix cannot be reset as an unexecuted remainder.
func TestFailedJobSuffix_RejectsUnsafeChains(t *testing.T) {
	node := types.NodeID("node")
	tests := []struct {
		name string
		jobs []Job
	}{
		{"empty", nil},
		{"success only", []Job{{ID: "a", Status: types.JobStatusSuccess}}},
		{"no owner", []Job{{ID: "a", Status: types.JobStatusError, RepoShaIn: testSHA}}},
		{"active", []Job{{ID: "a", Status: types.JobStatusRunning, NodeID: &node, RepoShaIn: testSHA}}},
		{"cancelled run", []Job{{ID: "a", Status: types.JobStatusCancelled, NodeID: &node, RepoShaIn: testSHA}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := failedJobSuffix(tt.jobs); !errors.Is(err, ErrRunResumeInvalid) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}
}

// Failed reset transactions preserve all job rows and resume history.
func TestRestartRun_FromFailedRollsBackInvalidStepConfiguration(t *testing.T) {
	ctx, db, fx, plans, jobs, _ := resumeFixture(t, 1)
	_, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans[:2]})
	if !errors.Is(err, ErrRunResumeInvalid) {
		t.Fatalf("expected invalid suffix: %v", err)
	}
	for _, old := range jobs {
		got, err := db.GetJob(ctx, old.ID)
		if err != nil || !reflect.DeepEqual(old, got) {
			t.Fatalf("failed transaction changed job: %+v %v", got, err)
		}
	}
	run, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if types.RunStats(run.Stats).ResumeCount() != 0 || run.Status == types.RunStatusRunning {
		t.Fatalf("failed transaction resumed run: %+v", run)
	}
}

// Restart waits for an in-flight execution writer before resetting that execution.
func TestRestartRun_FromFailedWaitsForExecutionWriter(t *testing.T) {
	ctx, db, fx, plans, jobs, _ := resumeFixture(t, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- db.WithJobExecution(ctx, jobs[1].ID, 0, func(Store, Job) error { close(entered); <-release; return nil })
	}()
	<-entered
	restartDone := make(chan error, 1)
	go func() {
		_, err := db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: 1, FromFailed: true, Jobs: plans})
		restartDone <- err
	}()
	select {
	case err := <-restartDone:
		close(release)
		<-writerDone
		t.Fatalf("restart passed active execution writer: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-restartDone; err != nil {
		t.Fatal(err)
	}
	if err := db.WithJobExecution(ctx, jobs[1].ID, 0, func(Store, Job) error { t.Fatal("stale writer ran"); return nil }); !errors.Is(err, ErrJobExecutionStale) {
		t.Fatalf("stale writer accepted: %v", err)
	}
}
