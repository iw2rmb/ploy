package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHookOnceLedgerQueries_UpsertSuccessAndSkipMarker(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/hooks-once", "main", "feature", []byte(`{"type":"hooks-once"}`))
	attempt := fx.RunRepo.Attempt

	successJob1 := createHookOnceTestJob(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, attempt, "pre-gate-hook-000")
	skipJob := createHookOnceTestJob(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, attempt, "pre-gate-hook-001")
	successJob2 := createHookOnceTestJob(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, attempt, "pre-gate-hook-002")

	hookHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := db.UpsertHookOnceSuccess(ctx, UpsertHookOnceSuccessParams{
		RunID:             fx.Run.ID,
		RepoID:            fx.RunRepo.RepoID,
		HookHash:          hookHash,
		FirstSuccessJobID: &successJob1.ID,
	}); err != nil {
		t.Fatalf("UpsertHookOnceSuccess(initial) failed: %v", err)
	}

	exists, err := db.HasHookOnceLedger(ctx, HasHookOnceLedgerParams{
		RunID:    fx.Run.ID,
		RepoID:   fx.RunRepo.RepoID,
		HookHash: hookHash,
	})
	if err != nil {
		t.Fatalf("HasHookOnceLedger() failed: %v", err)
	}
	if !exists {
		t.Fatal("HasHookOnceLedger()=false, want true after first success upsert")
	}

	row, err := db.GetHookOnceLedger(ctx, GetHookOnceLedgerParams{
		RunID:    fx.Run.ID,
		RepoID:   fx.RunRepo.RepoID,
		HookHash: hookHash,
	})
	if err != nil {
		t.Fatalf("GetHookOnceLedger() failed: %v", err)
	}
	if row.FirstSuccessJobID == nil || *row.FirstSuccessJobID != successJob1.ID {
		t.Fatalf("first_success_job_id=%v, want %s", row.FirstSuccessJobID, successJob1.ID)
	}
	if row.LastSuccessJobID == nil || *row.LastSuccessJobID != successJob1.ID {
		t.Fatalf("last_success_job_id=%v, want %s", row.LastSuccessJobID, successJob1.ID)
	}
	if row.LastSkipJobID != nil {
		t.Fatalf("last_skip_job_id=%v, want nil", row.LastSkipJobID)
	}
	if row.OnceSkipMarked {
		t.Fatal("once_skip_marked=true, want false before skip marker")
	}

	if err := db.MarkHookOnceSkipped(ctx, MarkHookOnceSkippedParams{
		RunID:         fx.Run.ID,
		RepoID:        fx.RunRepo.RepoID,
		HookHash:      hookHash,
		LastSkipJobID: &skipJob.ID,
	}); err != nil {
		t.Fatalf("MarkHookOnceSkipped() failed: %v", err)
	}

	if err := db.UpsertHookOnceSuccess(ctx, UpsertHookOnceSuccessParams{
		RunID:             fx.Run.ID,
		RepoID:            fx.RunRepo.RepoID,
		HookHash:          hookHash,
		FirstSuccessJobID: &successJob2.ID,
	}); err != nil {
		t.Fatalf("UpsertHookOnceSuccess(second) failed: %v", err)
	}

	row, err = db.GetHookOnceLedger(ctx, GetHookOnceLedgerParams{
		RunID:    fx.Run.ID,
		RepoID:   fx.RunRepo.RepoID,
		HookHash: hookHash,
	})
	if err != nil {
		t.Fatalf("GetHookOnceLedger(second) failed: %v", err)
	}
	if row.FirstSuccessJobID == nil || *row.FirstSuccessJobID != successJob1.ID {
		t.Fatalf("first_success_job_id after second upsert=%v, want stable %s", row.FirstSuccessJobID, successJob1.ID)
	}
	if row.LastSuccessJobID == nil || *row.LastSuccessJobID != successJob2.ID {
		t.Fatalf("last_success_job_id after second upsert=%v, want %s", row.LastSuccessJobID, successJob2.ID)
	}
	if row.LastSkipJobID == nil || *row.LastSkipJobID != skipJob.ID {
		t.Fatalf("last_skip_job_id=%v, want %s", row.LastSkipJobID, skipJob.ID)
	}
	if !row.OnceSkipMarked {
		t.Fatal("once_skip_marked=false, want true after skip marker")
	}

	rows, err := db.ListHookOnceLedgerByRunRepo(ctx, ListHookOnceLedgerByRunRepoParams{
		RunID:  fx.Run.ID,
		RepoID: fx.RunRepo.RepoID,
	})
	if err != nil {
		t.Fatalf("ListHookOnceLedgerByRunRepo() failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListHookOnceLedgerByRunRepo() rows=%d, want 1", len(rows))
	}
	if rows[0].HookHash != hookHash {
		t.Fatalf("listed hook_hash=%q, want %q", rows[0].HookHash, hookHash)
	}
}

func createHookOnceTestJob(
	t *testing.T,
	ctx context.Context,
	db Store,
	runID types.RunID,
	repoID types.RepoID,
	repoBaseRef string,
	attempt int32,
	name string,
) Job {
	t.Helper()

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      types.JobStatusSuccess,
		JobType:     types.JobTypeHook,
		JobImage:    "example.com/hook:latest",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s) failed: %v", name, err)
	}
	return job
}
