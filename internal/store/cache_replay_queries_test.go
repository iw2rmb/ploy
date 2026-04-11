package store

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestResolveReusableJobByCacheKey_FailedCandidateRequiresLogs(t *testing.T) {
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

	fixture := newV1Fixture(
		t,
		ctx,
		db,
		"https://example.com/cache-replay-log-guard-"+types.NewMigID().String()+".git",
		"main",
		"feature/cache-replay-log-guard",
		[]byte(`{"steps":[]}`),
	)

	cacheKey := "cache-key-" + types.NewJobID().String()
	failExitCode := int32(1)

	jobWithLogs, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fixture.Run.ID,
		RepoID:      fixture.MigRepo.RepoID,
		RepoBaseRef: fixture.MigRepo.BaseRef,
		Attempt:     fixture.RunRepo.Attempt,
		Name:        "candidate-with-logs",
		Status:      types.JobStatusCreated,
		JobType:     types.JobTypeMig,
		JobImage:    "alpine:3.20",
		Meta:        []byte(`{"kind":"mig"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(candidate-with-logs) failed: %v", err)
	}
	if err := db.UpdateJobCacheKey(ctx, UpdateJobCacheKeyParams{ID: jobWithLogs.ID, CacheKey: cacheKey}); err != nil {
		t.Fatalf("UpdateJobCacheKey(candidate-with-logs) failed: %v", err)
	}
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       jobWithLogs.ID,
		Status:   types.JobStatusFail,
		ExitCode: &failExitCode,
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(candidate-with-logs) failed: %v", err)
	}
	if _, err := db.CreateLog(ctx, CreateLogParams{
		RunID:   fixture.Run.ID,
		JobID:   &jobWithLogs.ID,
		ChunkNo: 1,
		DataSize: 1,
	}); err != nil {
		t.Fatalf("CreateLog(candidate-with-logs) failed: %v", err)
	}

	jobWithoutLogs, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fixture.Run.ID,
		RepoID:      fixture.MigRepo.RepoID,
		RepoBaseRef: fixture.MigRepo.BaseRef,
		Attempt:     fixture.RunRepo.Attempt,
		Name:        "candidate-without-logs",
		Status:      types.JobStatusCreated,
		JobType:     types.JobTypeMig,
		JobImage:    "alpine:3.20",
		Meta:        []byte(`{"kind":"mig"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(candidate-without-logs) failed: %v", err)
	}
	if err := db.UpdateJobCacheKey(ctx, UpdateJobCacheKeyParams{ID: jobWithoutLogs.ID, CacheKey: cacheKey}); err != nil {
		t.Fatalf("UpdateJobCacheKey(candidate-without-logs) failed: %v", err)
	}
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       jobWithoutLogs.ID,
		Status:   types.JobStatusFail,
		ExitCode: &failExitCode,
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(candidate-without-logs) failed: %v", err)
	}

	selected, err := db.ResolveReusableJobByCacheKey(ctx, ResolveReusableJobByCacheKeyParams{
		RepoID:   fixture.MigRepo.RepoID,
		JobType:  types.JobTypeMig,
		CacheKey: cacheKey,
	})
	if err != nil {
		t.Fatalf("ResolveReusableJobByCacheKey() failed: %v", err)
	}
	if selected.ID != jobWithLogs.ID {
		t.Fatalf("ResolveReusableJobByCacheKey() id=%s, want log-backed failed candidate %s", selected.ID, jobWithLogs.ID)
	}
}
