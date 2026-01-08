package store

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestScheduleNextJobNoRace verifies that concurrent calls to ScheduleNextJob
// do not double-schedule a job. When multiple schedulers call ScheduleNextJob
// for the same repo attempt simultaneously, only one should succeed in
// promoting a given job from Created to Queued.
//
// This test addresses roadmap/refactor/store.md:
//
//	"Make scheduling atomic (select + update) — stop scheduler races"
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/schedule-norace", "main", "feature", []byte(`{"type":"norace"}`))

	// Create multiple jobs in Created status (not Queued).
	const numJobs = 10
	jobIDs := make([]types.JobID, numJobs)
	for i := 0; i < numJobs; i++ {
		jobIDs[i] = types.NewJobID()
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobIDs[i],
			RunID:       fx.Run.ID,
			RepoID:      fx.ModRepo.ID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-norace-" + jobIDs[i].String(),
			ModType:     "",
			ModImage:    "",
			Status:      JobStatusCreated, // Created, not Queued
			StepIndex:   types.StepIndex(1000 * (i + 1)),
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Run concurrent schedulers trying to schedule the next job.
	const numSchedulers = 20
	var wg sync.WaitGroup
	results := make(chan *Job, numSchedulers)
	errors := make(chan error, numSchedulers)

	wg.Add(numSchedulers)
	for i := 0; i < numSchedulers; i++ {
		go func() {
			defer wg.Done()
			job, err := db.ScheduleNextJob(ctx, ScheduleNextJobParams{
				RunID:   fx.Run.ID,
				RepoID:  fx.ModRepo.ID,
				Attempt: fx.RunRepo.Attempt,
			})
			if err != nil {
				errors <- err
				return
			}
			results <- &job
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Collect results.
	var scheduledJobs []*Job
	var scheduleErrors []error
	for job := range results {
		scheduledJobs = append(scheduledJobs, job)
	}
	for err := range errors {
		scheduleErrors = append(scheduleErrors, err)
	}

	// Exactly one scheduler should succeed per job.
	// Since we have numJobs jobs and numSchedulers concurrent calls,
	// and all calls target the same repo attempt, we expect:
	// - Some calls succeed (up to numJobs)
	// - The rest fail with "no rows" (pgx.ErrNoRows)
	if len(scheduledJobs) == 0 {
		t.Fatal("Expected at least one job to be scheduled, got none")
	}

	// Verify no duplicate job IDs were scheduled.
	seenIDs := make(map[types.JobID]bool)
	for _, job := range scheduledJobs {
		if seenIDs[job.ID] {
			t.Errorf("Job %s was scheduled multiple times (race condition!)", job.ID)
		}
		seenIDs[job.ID] = true
	}

	// Verify all scheduled jobs are now in Queued status.
	for _, job := range scheduledJobs {
		if job.Status != JobStatusQueued {
			t.Errorf("Scheduled job %s has status %s, expected Queued", job.ID, job.Status)
		}
	}

	// Verify total scheduled count: should be min(numJobs, numSchedulers).
	expectedMax := numJobs
	if numSchedulers < numJobs {
		expectedMax = numSchedulers
	}
	if len(scheduledJobs) > expectedMax {
		t.Errorf("Too many jobs scheduled: got %d, expected at most %d", len(scheduledJobs), expectedMax)
	}

	t.Logf("Concurrent schedulers: %d, Jobs created: %d, Jobs scheduled: %d, Errors (no rows): %d",
		numSchedulers, numJobs, len(scheduledJobs), len(scheduleErrors))
}

// TestScheduleNextJobSequential verifies that sequential calls to ScheduleNextJob
// correctly schedule jobs in step_index order.
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
	stepIndices := []types.StepIndex{5000, 1000, 3000, 2000, 4000} // Not in order

	for i := 0; i < numJobs; i++ {
		jobIDs[i] = types.NewJobID()
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobIDs[i],
			RunID:       fx.Run.ID,
			RepoID:      fx.ModRepo.ID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-seq-" + jobIDs[i].String(),
			ModType:     "",
			ModImage:    "",
			Status:      JobStatusCreated,
			StepIndex:   stepIndices[i],
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Build expected order: jobs sorted by step_index ASC.
	type jobWithIndex struct {
		id    types.JobID
		index types.StepIndex
	}
	jobsWithIndex := make([]jobWithIndex, numJobs)
	for i := 0; i < numJobs; i++ {
		jobsWithIndex[i] = jobWithIndex{id: jobIDs[i], index: stepIndices[i]}
	}
	// Sort by step_index.
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
			RepoID:  fx.ModRepo.ID,
			Attempt: fx.RunRepo.Attempt,
		})
		if err != nil {
			t.Fatalf("ScheduleNextJob(%d) failed: %v", i, err)
		}
		scheduledOrder = append(scheduledOrder, job.ID)
	}

	// Verify order matches expected (sorted by step_index ASC).
	for i := 0; i < numJobs; i++ {
		if scheduledOrder[i] != expectedOrder[i] {
			t.Errorf("ScheduleNextJob order mismatch at position %d: got %s, want %s",
				i, scheduledOrder[i], expectedOrder[i])
		}
	}

	// Verify no more jobs to schedule.
	_, err = db.ScheduleNextJob(ctx, ScheduleNextJobParams{
		RunID:   fx.Run.ID,
		RepoID:  fx.ModRepo.ID,
		Attempt: fx.RunRepo.Attempt,
	})
	if err == nil {
		t.Error("Expected ScheduleNextJob to fail when no Created jobs remain, but it succeeded")
	}

	t.Logf("Jobs scheduled in expected step_index order")
}
