package store

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

// TestNewStore verifies that Store creation works with a valid DSN.
// This test is skipped if PLOY_TEST_PG_DSN is not set, following the pattern
// of integration tests that require external dependencies.
func TestNewStore(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store initialization test")
	}

	ctx := context.Background()
	store, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer store.Close()
}

// TestNewStore_InvalidDSN verifies that Store creation fails gracefully with an invalid DSN.
func TestNewStore_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewStore(ctx, "invalid-dsn")
	if err == nil {
		t.Fatal("NewStore() should have failed with invalid DSN")
	}
}

func TestCreateRun_RoundTrip_V1(t *testing.T) {
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

	createdBy := "test-user"

	specID := types.NewSpecID().String()
	spec, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID,
		Name:      "test-spec",
		Spec:      []byte(`{"type":"test"}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	modID := types.NewModID().String()
	_, err = db.CreateMod(ctx, CreateModParams{
		ID:        modID,
		Name:      "test-mod-" + modID,
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	runID := types.NewRunID().String()
	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:        runID,
		ModID:     modID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}
	if run.Status != RunStatusStarted {
		t.Fatalf("CreateRun() status=%q, want %q", run.Status, RunStatusStarted)
	}

	fetched, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if fetched.ModID != modID {
		t.Fatalf("GetRun().mod_id=%q, want %q", fetched.ModID, modID)
	}
	if fetched.SpecID != spec.ID {
		t.Fatalf("GetRun().spec_id=%q, want %q", fetched.SpecID, spec.ID)
	}

	runs, err := db.ListRuns(ctx, ListRunsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListRuns() failed: %v", err)
	}
	found := false
	for _, r := range runs {
		if r.ID == run.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ListRuns() to include created run")
	}
}

func TestRunRepo_CRUDAndStateTransitions_V1(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", "feature/a", []byte(`{"type":"batch"}`))

	if fx.RunRepo.Status != RunRepoStatusQueued {
		t.Fatalf("CreateRunRepo() status=%q, want %q", fx.RunRepo.Status, RunRepoStatusQueued)
	}
	if fx.RunRepo.Attempt != 1 {
		t.Fatalf("CreateRunRepo() attempt=%d, want 1", fx.RunRepo.Attempt)
	}

	// Add a second repo for the mod and run.
	modRepo2ID := types.NewModRepoID().String()
	_, err = db.CreateModRepo(ctx, CreateModRepoParams{
		ID:        modRepo2ID,
		ModID:     fx.Mod.ID,
		RepoUrl:   "https://github.com/org/repo-b",
		BaseRef:   "main",
		TargetRef: "feature/b",
	})
	if err != nil {
		t.Fatalf("CreateModRepo() for repo-b failed: %v", err)
	}
	rr2, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		ModID:         fx.Mod.ID,
		RunID:         fx.Run.ID,
		RepoID:        modRepo2ID,
		RepoBaseRef:   "main",
		RepoTargetRef: "feature/b",
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() for repo-b failed: %v", err)
	}

	repos, err := db.ListRunReposByRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("ListRunReposByRun() failed: %v", err)
	}
	if len(repos) < 2 {
		t.Fatalf("expected at least 2 run_repos, got %d", len(repos))
	}

	// Transition repo-b: Queued -> Running -> Success.
	if err := db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
		RunID:  rr2.RunID,
		RepoID: rr2.RepoID,
		Status: RunRepoStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateRunRepoStatus() to Running failed: %v", err)
	}
	updated, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() failed: %v", err)
	}
	if updated.Status != RunRepoStatusRunning {
		t.Fatalf("run_repo status=%q, want %q", updated.Status, RunRepoStatusRunning)
	}
	if !updated.StartedAt.Valid {
		t.Fatal("expected started_at to be set for Running repo")
	}
	if err := db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
		RunID:  rr2.RunID,
		RepoID: rr2.RepoID,
		Status: RunRepoStatusSuccess,
	}); err != nil {
		t.Fatalf("UpdateRunRepoStatus() to Success failed: %v", err)
	}
	final, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() failed: %v", err)
	}
	if final.Status != RunRepoStatusSuccess {
		t.Fatalf("run_repo status=%q, want %q", final.Status, RunRepoStatusSuccess)
	}
	if !final.FinishedAt.Valid {
		t.Fatal("expected finished_at to be set for terminal repo")
	}

	// Attempt increment resets repo state.
	if err := db.IncrementRunRepoAttempt(ctx, IncrementRunRepoAttemptParams{RunID: rr2.RunID, RepoID: rr2.RepoID}); err != nil {
		t.Fatalf("IncrementRunRepoAttempt() failed: %v", err)
	}
	retry, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() after increment failed: %v", err)
	}
	if retry.Attempt != 2 {
		t.Fatalf("attempt=%d, want 2", retry.Attempt)
	}
	if retry.Status != RunRepoStatusQueued {
		t.Fatalf("status=%q, want %q", retry.Status, RunRepoStatusQueued)
	}

	msg := "boom"
	if err := db.UpdateRunRepoError(ctx, UpdateRunRepoErrorParams{RunID: rr2.RunID, RepoID: rr2.RepoID, LastError: &msg}); err != nil {
		t.Fatalf("UpdateRunRepoError() failed: %v", err)
	}
	got, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() after error failed: %v", err)
	}
	if got.LastError == nil || *got.LastError != msg {
		t.Fatalf("last_error=%v, want %q", got.LastError, msg)
	}

	if err := db.DeleteRunRepo(ctx, DeleteRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID}); err != nil {
		t.Fatalf("DeleteRunRepo() failed: %v", err)
	}
	_, err = db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err == nil {
		t.Fatal("expected GetRunRepo() after delete to fail")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}
