package store

import (
	"context"
	"net/netip"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestCompleteJobDurationNeverNull verifies that duration_ms is always non-null
// after job completion, even if started_at is NULL (defensive computation).
//
// This test creates a job without claiming it (started_at remains NULL) and then
// completes it using UpdateJobCompletion and UpdateJobCompletionWithMeta.
// The duration_ms must always be set to a valid non-negative value (0 when
// started_at is NULL, actual duration otherwise).
//
// Requires PLOY_TEST_DB_DSN to be set with a test database.
func TestCompleteJobDurationNeverNull(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Create v1 fixture (mig, spec, mod_repo, run, run_repo).
	fixture := newV1Fixture(t, ctx, db,
		"https://github.com/iw2rmb/ploy-duration-test.git",
		"main",
		"feature",
		[]byte(`{"steps":[]}`),
	)

	t.Run("UpdateJobCompletion with NULL started_at", func(t *testing.T) {
		// Create a job in Queued status (started_at is NULL).
		jobID := types.NewJobID()
		job, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       fixture.Run.ID,
			RepoID:      fixture.MigRepo.RepoID,
			RepoBaseRef: fixture.RunRepo.RepoBaseRef,
			Attempt:     fixture.RunRepo.Attempt,
			Name:        "test-job-completion-1",
			Status:      types.JobStatusQueued,
			JobType:     "mig",
			JobImage:    "test-image",
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob() failed: %v", err)
		}
		defer func() { _ = db.DeleteJob(ctx, job.ID) }()

		// Verify started_at is NULL after creation.
		if job.StartedAt.Valid {
			t.Fatalf("expected started_at to be NULL after CreateJob, got %v", job.StartedAt.Time)
		}

		// Complete the job without claiming (started_at remains NULL).
		exitCode := int32(0)
		err = db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
			ID:       job.ID,
			Status:   types.JobStatusSuccess,
			ExitCode: &exitCode,
		})
		if err != nil {
			t.Fatalf("UpdateJobCompletion() failed: %v", err)
		}

		// Fetch the job and verify duration_ms is set (not null).
		completed, err := db.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetJob() failed: %v", err)
		}

		// duration_ms should be 0 when started_at was NULL (defensive computation).
		if completed.DurationMs != 0 {
			t.Errorf("expected duration_ms=0 when started_at is NULL, got %d", completed.DurationMs)
		}

		// Verify finished_at is set.
		if !completed.FinishedAt.Valid {
			t.Error("expected finished_at to be set after UpdateJobCompletion")
		}
	})

	t.Run("UpdateJobCompletionWithMeta with NULL started_at", func(t *testing.T) {
		// Create a job in Queued status (started_at is NULL).
		jobID := types.NewJobID()
		job, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       fixture.Run.ID,
			RepoID:      fixture.MigRepo.RepoID,
			RepoBaseRef: fixture.RunRepo.RepoBaseRef,
			Attempt:     fixture.RunRepo.Attempt,
			Name:        "test-job-completion-2",
			Status:      types.JobStatusQueued,
			JobType:     "mig",
			JobImage:    "test-image",
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob() failed: %v", err)
		}
		defer func() { _ = db.DeleteJob(ctx, job.ID) }()

		// Verify started_at is NULL after creation.
		if job.StartedAt.Valid {
			t.Fatalf("expected started_at to be NULL after CreateJob, got %v", job.StartedAt.Time)
		}

		// Complete the job with meta, without claiming (started_at remains NULL).
		exitCode := int32(0)
		err = db.UpdateJobCompletionWithMeta(ctx, UpdateJobCompletionWithMetaParams{
			ID:       job.ID,
			Status:   types.JobStatusSuccess,
			ExitCode: &exitCode,
			Meta:     []byte(`{"completed": true}`),
		})
		if err != nil {
			t.Fatalf("UpdateJobCompletionWithMeta() failed: %v", err)
		}

		// Fetch the job and verify duration_ms is set (not null).
		completed, err := db.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetJob() failed: %v", err)
		}

		// duration_ms should be 0 when started_at was NULL (defensive computation).
		if completed.DurationMs != 0 {
			t.Errorf("expected duration_ms=0 when started_at is NULL, got %d", completed.DurationMs)
		}

		// Verify finished_at is set.
		if !completed.FinishedAt.Valid {
			t.Error("expected finished_at to be set after UpdateJobCompletionWithMeta")
		}
	})

	t.Run("UpdateJobStatus to Running sets started_at", func(t *testing.T) {
		jobID := types.NewJobID()
		job, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       fixture.Run.ID,
			RepoID:      fixture.MigRepo.RepoID,
			RepoBaseRef: fixture.RunRepo.RepoBaseRef,
			Attempt:     fixture.RunRepo.Attempt,
			Name:        "test-job-running-started-at",
			Status:      types.JobStatusQueued,
			JobType:     "mig",
			JobImage:    "test-image",
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob() failed: %v", err)
		}
		defer func() { _ = db.DeleteJob(ctx, job.ID) }()

		if job.StartedAt.Valid {
			t.Fatalf("expected started_at to be NULL after CreateJob, got %v", job.StartedAt.Time)
		}

		if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{
			ID:         job.ID,
			Status:     types.JobStatusRunning,
			StartedAt:  pgtype.Timestamptz{}, // invalid/null input; store should set now() on transition
			FinishedAt: pgtype.Timestamptz{},
			DurationMs: 0,
		}); err != nil {
			t.Fatalf("UpdateJobStatus() failed: %v", err)
		}

		running, err := db.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetJob() failed: %v", err)
		}
		if running.Status != types.JobStatusRunning {
			t.Fatalf("status=%q, want %q", running.Status, types.JobStatusRunning)
		}
		if !running.StartedAt.Valid {
			t.Fatal("expected started_at to be set after UpdateJobStatus(Running)")
		}
	})

	t.Run("UpdateJobCompletion with valid started_at", func(t *testing.T) {
		// Cancel any pre-existing Queued jobs so ClaimJob below picks only our job.
		if _, err := db.Pool().Exec(ctx, "UPDATE jobs SET status = 'Cancelled' WHERE status = 'Queued'"); err != nil {
			t.Fatalf("cancel queued jobs: %v", err)
		}

		// Create and claim a job (started_at will be set during claim).
		jobID := types.NewJobID()
		job, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       fixture.Run.ID,
			RepoID:      fixture.MigRepo.RepoID,
			RepoBaseRef: fixture.RunRepo.RepoBaseRef,
			Attempt:     fixture.RunRepo.Attempt,
			Name:        "test-job-completion-3",
			Status:      types.JobStatusQueued,
			JobType:     "mig",
			JobImage:    "test-image",
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob() failed: %v", err)
		}
		defer func() { _ = db.DeleteJob(ctx, job.ID) }()

		// Create a node to claim with.
		nodeID := types.NodeID(types.NewNodeKey())
		ipAddr, _ := netip.ParseAddr("192.0.2.1")
		_, err = db.CreateNode(ctx, CreateNodeParams{
			ID:          nodeID,
			Name:        "test-node-" + string(nodeID),
			IpAddress:   ipAddr,
			Concurrency: 1,
		})
		if err != nil {
			t.Fatalf("CreateNode() failed: %v", err)
		}
		defer func() { _ = db.DeleteNode(ctx, nodeID) }()

		// Run is already Started (created in newV1Fixture).
		// Claim the job (sets started_at = now()).
		claimed, err := db.ClaimJob(ctx, nodeID)
		if err != nil {
			t.Fatalf("ClaimJob() failed: %v", err)
		}
		if claimed.ID != job.ID {
			t.Fatalf("ClaimJob() returned different job: got=%v want=%v", claimed.ID, job.ID)
		}
		if !claimed.StartedAt.Valid {
			t.Fatal("expected started_at to be set after ClaimJob")
		}

		// Complete the job (duration_ms should be computed from started_at).
		exitCode := int32(0)
		err = db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
			ID:       claimed.ID,
			Status:   types.JobStatusSuccess,
			ExitCode: &exitCode,
		})
		if err != nil {
			t.Fatalf("UpdateJobCompletion() failed: %v", err)
		}

		// Fetch the job and verify duration_ms is non-negative.
		completed, err := db.GetJob(ctx, claimed.ID)
		if err != nil {
			t.Fatalf("GetJob() failed: %v", err)
		}

		// duration_ms should be >= 0 when started_at was set.
		if completed.DurationMs < 0 {
			t.Errorf("expected duration_ms >= 0, got %d", completed.DurationMs)
		}

		// Verify finished_at is set.
		if !completed.FinishedAt.Valid {
			t.Error("expected finished_at to be set after UpdateJobCompletion")
		}
	})
}
