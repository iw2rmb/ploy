package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

// TestScheduleNextJobNoRace verifies that concurrent calls to ScheduleNextJob
// do not double-schedule a job. When multiple schedulers call ScheduleNextJob
// for the same repo attempt simultaneously, only one should succeed in
// promoting a given job from Created to Queued.
//
// The fix uses FOR UPDATE SKIP LOCKED in the subquery and a status predicate
// in the update to ensure concurrent schedulers cannot race.
//
// Requires PLOY_TEST_PG_DSN to be set with a test database.
func TestScheduleNextJobNoRace(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	if db.Pool().Config().MaxConns < 2 {
		t.Skipf("pgxpool max_conns=%d; need >=2 to exercise concurrent schedulers", db.Pool().Config().MaxConns)
	}

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/schedule-norace", "main", "feature", []byte(`{"type":"norace"}`))

	// Create multiple jobs in Created status (not Queued).
	const numJobs = 10
	for i := 0; i < numJobs; i++ {
		jobID := types.NewJobID()
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       fx.Run.ID,
			RepoID:      fx.MigRepo.RepoID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-norace-" + jobID.String(),
			JobType:     "",
			JobImage:    "",
			Status:      JobStatusCreated, // Created, not Queued
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Run concurrent schedulers trying to schedule the next job.
	const numSchedulers = numJobs * 2
	start := make(chan struct{})
	var wg sync.WaitGroup
	scheduled := make(chan types.JobID, numSchedulers)
	errs := make(chan error, numSchedulers)

	wg.Add(numSchedulers)
	for i := 0; i < numSchedulers; i++ {
		go func() {
			defer wg.Done()
			<-start
			job, err := db.ScheduleNextJob(ctx, ScheduleNextJobParams{
				RunID:   fx.Run.ID,
				RepoID:  fx.MigRepo.RepoID,
				Attempt: fx.RunRepo.Attempt,
			})
			if err != nil {
				errs <- err
				return
			}
			scheduled <- job.ID
		}()
	}

	close(start)
	wg.Wait()
	close(scheduled)
	close(errs)

	// Collect results.
	var scheduledIDs []types.JobID
	var scheduleErrors []error
	for id := range scheduled {
		scheduledIDs = append(scheduledIDs, id)
	}
	for err := range errs {
		scheduleErrors = append(scheduleErrors, err)
	}

	if len(scheduledIDs) != numJobs {
		t.Fatalf("scheduled=%d, want %d (every Created job should be promoted)", len(scheduledIDs), numJobs)
	}
	if len(scheduleErrors) != (numSchedulers - numJobs) {
		t.Fatalf("errors=%d, want %d (extra schedulers should get no rows)", len(scheduleErrors), numSchedulers-numJobs)
	}
	for _, err := range scheduleErrors {
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("unexpected ScheduleNextJob error: %v", err)
		}
	}

	// Verify no duplicate job IDs were scheduled.
	seenIDs := make(map[types.JobID]struct{}, numJobs)
	for _, id := range scheduledIDs {
		if _, ok := seenIDs[id]; ok {
			t.Fatalf("job %s scheduled multiple times (scheduler race)", id)
		}
		seenIDs[id] = struct{}{}
	}

	// Verify all scheduled jobs are now in Queued status in the DB.
	for id := range seenIDs {
		job, err := db.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("GetJob(%s) failed: %v", id, err)
		}
		if job.Status != JobStatusQueued {
			t.Fatalf("job %s status=%s, want %s", id, job.Status, JobStatusQueued)
		}
	}

	// Verify no Created jobs remain and exactly numJobs are Queued for the repo attempt.
	rows, err := db.CountJobsByRunRepoAttemptGroupByStatus(ctx, CountJobsByRunRepoAttemptGroupByStatusParams{
		RunID:   fx.Run.ID,
		RepoID:  fx.MigRepo.RepoID,
		Attempt: fx.RunRepo.Attempt,
	})
	if err != nil {
		t.Fatalf("CountJobsByRunRepoAttemptGroupByStatus() failed: %v", err)
	}
	counts := make(map[JobStatus]int32, len(rows))
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	if got := counts[JobStatusQueued]; got != numJobs {
		t.Fatalf("Queued jobs=%d, want %d", got, numJobs)
	}
	if got := counts[JobStatusCreated]; got != 0 {
		t.Fatalf("Created jobs=%d, want 0", got)
	}

	t.Logf("Concurrent schedulers: %d, Jobs created: %d, Jobs scheduled: %d, Errors (no rows): %d",
		numSchedulers, numJobs, len(scheduledIDs), len(scheduleErrors))
}

// TestScheduleNextJobSequential verifies that sequential calls to ScheduleNextJob
// correctly schedule jobs in next_id order.
func TestScheduleNextJobSequential(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/schedule-seq", "main", "feature", []byte(`{"type":"sequential"}`))

	// Create jobs with distinct step indices.
	const numJobs = 5
	jobIDs := make([]types.JobID, numJobs)
	stepIndices := []float64{5000, 1000, 3000, 2000, 4000} // Not in order

	for i := 0; i < numJobs; i++ {
		jobIDs[i] = types.NewJobID()
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobIDs[i],
			RunID:       fx.Run.ID,
			RepoID:      fx.MigRepo.RepoID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-seq-" + jobIDs[i].String(),
			JobType:     "",
			JobImage:    "",
			Status:      JobStatusCreated,
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Build expected order: jobs sorted by next_id ASC.
	type jobWithIndex struct {
		id    types.JobID
		index float64
	}
	jobsWithIndex := make([]jobWithIndex, numJobs)
	for i := 0; i < numJobs; i++ {
		jobsWithIndex[i] = jobWithIndex{id: jobIDs[i], index: stepIndices[i]}
	}
	// Sort by next_id.
	for i := 0; i < len(jobsWithIndex); i++ {
		for j := i + 1; j < len(jobsWithIndex); j++ {
			if jobsWithIndex[j].index < jobsWithIndex[i].index {
				jobsWithIndex[i], jobsWithIndex[j] = jobsWithIndex[j], jobsWithIndex[i]
			}
		}
	}
	expectedOrder := make([]types.JobID, numJobs)
	for i, j := range jobsWithIndex {
		expectedOrder[i] = j.id
	}

	// Schedule jobs one by one and verify order.
	scheduledOrder := make([]types.JobID, 0, numJobs)
	for i := 0; i < numJobs; i++ {
		job, err := db.ScheduleNextJob(ctx, ScheduleNextJobParams{
			RunID:   fx.Run.ID,
			RepoID:  fx.MigRepo.RepoID,
			Attempt: fx.RunRepo.Attempt,
		})
		if err != nil {
			t.Fatalf("ScheduleNextJob(%d) failed: %v", i, err)
		}
		scheduledOrder = append(scheduledOrder, job.ID)
	}

	// Verify order matches expected (sorted by next_id ASC).
	for i := 0; i < numJobs; i++ {
		if scheduledOrder[i] != expectedOrder[i] {
			t.Errorf("ScheduleNextJob order mismatch at position %d: got %s, want %s",
				i, scheduledOrder[i], expectedOrder[i])
		}
	}

	// Verify no more jobs to schedule.
	_, err = db.ScheduleNextJob(ctx, ScheduleNextJobParams{
		RunID:   fx.Run.ID,
		RepoID:  fx.MigRepo.RepoID,
		Attempt: fx.RunRepo.Attempt,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected no rows when no Created jobs remain, got: %v", err)
	}

	t.Logf("Jobs scheduled in expected next_id order")
}
