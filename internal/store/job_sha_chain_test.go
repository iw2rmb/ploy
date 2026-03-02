package store

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestUpdateJobCompletion_PropagatesRepoSHAOutToNextJob(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	fx := newV1Fixture(t, ctx, db,
		"https://github.com/iw2rmb/ploy-sha-chain-test.git",
		"main",
		"feature",
		[]byte(`{"steps":[]}`),
	)

	nextJobID := types.NewJobID()
	nextJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          nextJobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "sha-chain-next",
		Status:      JobStatusCreated,
		JobType:     "mig",
		JobImage:    "img",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(next) failed: %v", err)
	}
	defer func() { _ = db.DeleteJob(ctx, nextJob.ID) }()

	currJobID := types.NewJobID()
	currJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          currJobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "sha-chain-current",
		Status:      JobStatusRunning,
		JobType:     "mig",
		JobImage:    "img",
		NextID:      &nextJobID,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(current) failed: %v", err)
	}
	defer func() { _ = db.DeleteJob(ctx, currJob.ID) }()

	shaOut := "89abcdef0123456789abcdef0123456789abcdef"
	exitCode := int32(0)
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:         currJob.ID,
		Status:     JobStatusSuccess,
		ExitCode:   &exitCode,
		RepoShaOut: shaOut,
	}); err != nil {
		t.Fatalf("UpdateJobCompletion() failed: %v", err)
	}

	updatedCurrent, err := db.GetJob(ctx, currJob.ID)
	if err != nil {
		t.Fatalf("GetJob(current) failed: %v", err)
	}
	if updatedCurrent.RepoShaOut != shaOut {
		t.Fatalf("current.repo_sha_out=%q, want %q", updatedCurrent.RepoShaOut, shaOut)
	}
	if updatedCurrent.RepoShaOut8 != shaOut[:8] {
		t.Fatalf("current.repo_sha_out8=%q, want %q", updatedCurrent.RepoShaOut8, shaOut[:8])
	}

	updatedNext, err := db.GetJob(ctx, nextJob.ID)
	if err != nil {
		t.Fatalf("GetJob(next) failed: %v", err)
	}
	if updatedNext.RepoShaIn != shaOut {
		t.Fatalf("next.repo_sha_in=%q, want %q", updatedNext.RepoShaIn, shaOut)
	}
	if updatedNext.RepoShaIn8 != shaOut[:8] {
		t.Fatalf("next.repo_sha_in8=%q, want %q", updatedNext.RepoShaIn8, shaOut[:8])
	}
}

func TestUpdateJobCompletion_PropagationIsAtomic(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	fx := newV1Fixture(t, ctx, db,
		"https://github.com/iw2rmb/ploy-sha-chain-atomic.git",
		"main",
		"feature",
		[]byte(`{"steps":[]}`),
	)

	nextJobID := types.NewJobID()
	nextJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          nextJobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "sha-atomic-next",
		Status:      JobStatusCreated,
		JobType:     "mig",
		JobImage:    "img",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(next) failed: %v", err)
	}
	defer func() { _ = db.DeleteJob(ctx, nextJob.ID) }()

	currJobID := types.NewJobID()
	currJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          currJobID,
		RunID:       fx.Run.ID,
		RepoID:      fx.MigRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "sha-atomic-current",
		Status:      JobStatusRunning,
		JobType:     "mig",
		JobImage:    "img",
		NextID:      &nextJobID,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(current) failed: %v", err)
	}
	defer func() { _ = db.DeleteJob(ctx, currJob.ID) }()

	shaOut := "fedcba9876543210fedcba9876543210fedcba98"
	constraintName := "jobs_repo_sha_in_block_test"
	if _, err := db.Pool().Exec(ctx, fmt.Sprintf("ALTER TABLE ploy.jobs DROP CONSTRAINT IF EXISTS %s", constraintName)); err != nil {
		t.Fatalf("drop stale test constraint failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, fmt.Sprintf(
		"ALTER TABLE ploy.jobs ADD CONSTRAINT %s CHECK (repo_sha_in <> '%s')",
		constraintName,
		shaOut,
	)); err != nil {
		t.Fatalf("add test constraint failed: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(ctx, fmt.Sprintf("ALTER TABLE ploy.jobs DROP CONSTRAINT IF EXISTS %s", constraintName))
	}()

	exitCode := int32(0)
	err = db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:         currJob.ID,
		Status:     JobStatusSuccess,
		ExitCode:   &exitCode,
		RepoShaOut: shaOut,
	})
	if err == nil {
		t.Fatal("expected UpdateJobCompletion to fail when propagation update violates constraint")
	}

	reloadedCurrent, err := db.GetJob(ctx, currJob.ID)
	if err != nil {
		t.Fatalf("GetJob(current) failed: %v", err)
	}
	if reloadedCurrent.RepoShaOut != "" {
		t.Fatalf("expected current.repo_sha_out rollback to empty, got %q", reloadedCurrent.RepoShaOut)
	}

	reloadedNext, err := db.GetJob(ctx, nextJob.ID)
	if err != nil {
		t.Fatalf("GetJob(next) failed: %v", err)
	}
	if reloadedNext.RepoShaIn != "" {
		t.Fatalf("expected next.repo_sha_in to remain empty, got %q", reloadedNext.RepoShaIn)
	}
}
