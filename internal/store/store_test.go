package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestNewStore verifies that Store creation works with a valid DSN.
// This test is skipped if PLOY_TEST_DB_DSN is not set, following the pattern
// of integration tests that require external dependencies.
func TestNewStore(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store initialization test")
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

// TestConnectSearchPath verifies that NewStore sets search_path so unqualified
// table names resolve to the ploy schema, regardless of DSN formatting.
func TestConnectSearchPath(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping search_path test")
	}

	// Ensure the test does not rely on DSN formatting (e.g. `search_path=` in the DSN).
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams != nil {
		delete(cfg.ConnConfig.RuntimeParams, "search_path")
		// If the DSN specifies search_path via `options`, drop options entirely so this
		// test is not coupled to any DSN-level search_path formatting.
		if opt, ok := cfg.ConnConfig.RuntimeParams["options"]; ok && strings.Contains(opt, "search_path") {
			delete(cfg.ConnConfig.RuntimeParams, "options")
		}
	}
	dsn = cfg.ConnConfig.ConnString()
	cfg2, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse sanitized dsn: %v", err)
	}
	if cfg2.ConnConfig.RuntimeParams != nil {
		if _, ok := cfg2.ConnConfig.RuntimeParams["search_path"]; ok {
			t.Fatalf("sanitized dsn must not include search_path runtime param")
		}
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Query the current search_path to verify it includes "ploy".
	var searchPath string
	err = db.Pool().QueryRow(ctx, "SHOW search_path").Scan(&searchPath)
	if err != nil {
		t.Fatalf("SHOW search_path failed: %v", err)
	}
	if searchPath != "ploy, public" {
		t.Fatalf("search_path=%q, want %q", searchPath, "ploy, public")
	}

	// Verify that an unqualified query to a ploy schema table works.
	// Use "runs" which is defined in ploy schema (internal/store/schema.sql).
	_, err = db.Pool().Exec(ctx, "SELECT id FROM runs LIMIT 0")
	if err != nil {
		t.Fatalf("unqualified query to ploy.runs failed: %v", err)
	}
}

func TestCreateRun_RoundTrip_V1(t *testing.T) {
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

	createdBy := "test-user"

	specID := types.NewSpecID()
	spec, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID,
		Name:      "test-spec",
		Spec:      []byte(`{"type":"test"}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	modID := types.NewMigID()
	_, err = db.CreateMig(ctx, CreateMigParams{
		ID:        modID,
		Name:      "test-mig-" + modID.String(),
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	runID := types.NewRunID()
	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:        runID,
		MigID:     modID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}
	if run.Status != types.RunStatusStarted {
		t.Fatalf("CreateRun() status=%q, want %q", run.Status, types.RunStatusStarted)
	}

	fetched, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if fetched.MigID != modID {
		t.Fatalf("GetRun().mig_id=%q, want %q", fetched.MigID, modID)
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", "feature/a", []byte(`{"type":"batch"}`))

	if fx.RunRepo.Status != types.RunRepoStatusQueued {
		t.Fatalf("CreateRunRepo() status=%q, want %q", fx.RunRepo.Status, types.RunRepoStatusQueued)
	}
	if fx.RunRepo.Attempt != 1 {
		t.Fatalf("CreateRunRepo() attempt=%d, want 1", fx.RunRepo.Attempt)
	}

	// Add a second repo for the mig and run.
	modRepo2ID := types.NewMigRepoID()
	modRepo2, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        modRepo2ID,
		MigID:     fx.Mig.ID,
		Url:       "https://github.com/org/repo-b",
		BaseRef:   "main",
		TargetRef: "feature/b",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() for repo-b failed: %v", err)
	}
	rr2, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:           fx.Mig.ID,
		RunID:           fx.Run.ID,
		RepoID:          modRepo2.RepoID,
		RepoBaseRef:     "main",
		RepoTargetRef:   "feature/b",
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
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
		Status: types.RunRepoStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateRunRepoStatus() to Running failed: %v", err)
	}
	updated, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() failed: %v", err)
	}
	if updated.Status != types.RunRepoStatusRunning {
		t.Fatalf("run_repo status=%q, want %q", updated.Status, types.RunRepoStatusRunning)
	}
	if !updated.StartedAt.Valid {
		t.Fatal("expected started_at to be set for Running repo")
	}
	if err := db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
		RunID:  rr2.RunID,
		RepoID: rr2.RepoID,
		Status: types.RunRepoStatusSuccess,
	}); err != nil {
		t.Fatalf("UpdateRunRepoStatus() to Success failed: %v", err)
	}
	final, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: rr2.RunID, RepoID: rr2.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo() failed: %v", err)
	}
	if final.Status != types.RunRepoStatusSuccess {
		t.Fatalf("run_repo status=%q, want %q", final.Status, types.RunRepoStatusSuccess)
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
	if retry.Status != types.RunRepoStatusQueued {
		t.Fatalf("status=%q, want %q", retry.Status, types.RunRepoStatusQueued)
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

func TestListRunReposWithURLByRun_ReturnsRepoURLAndOrdering_V1(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", "feature/a", []byte(`{"type":"batch"}`))

	modRepo2ID := types.NewMigRepoID()
	modRepo2, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        modRepo2ID,
		MigID:     fx.Mig.ID,
		Url:       "https://github.com/org/repo-b",
		BaseRef:   "main",
		TargetRef: "feature/b",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() for repo-b failed: %v", err)
	}

	_, err = db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:           fx.Mig.ID,
		RunID:           fx.Run.ID,
		RepoID:          modRepo2.RepoID,
		RepoBaseRef:     modRepo2.BaseRef,
		RepoTargetRef:   modRepo2.TargetRef,
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() for repo-b failed: %v", err)
	}

	rows, err := db.ListRunReposWithURLByRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("ListRunReposWithURLByRun() failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 run repos with urls, got %d", len(rows))
	}

	expectedURLByRepoID := map[types.RepoID]string{
		fx.MigRepo.RepoID: "https://github.com/org/repo-a",
		modRepo2.RepoID:   "https://github.com/org/repo-b",
	}

	seen := map[types.RepoID]bool{}
	for i, row := range rows {
		if row.RunID != fx.Run.ID {
			t.Fatalf("row[%d] run_id=%q, want %q", i, row.RunID, fx.Run.ID)
		}
		if row.RepoUrl == "" {
			t.Fatalf("row[%d] repo_url is empty", i)
		}

		wantURL, ok := expectedURLByRepoID[row.RepoID]
		if !ok {
			t.Fatalf("row[%d] returned unexpected repo_id=%q", i, row.RepoID)
		}
		if row.RepoUrl != wantURL {
			t.Fatalf("row[%d] repo_url=%q, want %q", i, row.RepoUrl, wantURL)
		}
		seen[row.RepoID] = true

		if i == 0 {
			continue
		}
		prev := rows[i-1]
		if prev.CreatedAt.Time.After(row.CreatedAt.Time) {
			t.Fatalf("rows not ordered by created_at ASC at index %d", i)
		}
		if prev.CreatedAt.Time.Equal(row.CreatedAt.Time) && string(prev.RepoID) > string(row.RepoID) {
			t.Fatalf("rows with equal created_at not ordered by repo_id ASC at index %d", i)
		}
	}

	if !seen[fx.MigRepo.RepoID] {
		t.Fatalf("expected repo_id %q in results", fx.MigRepo.RepoID)
	}
	if !seen[modRepo2.RepoID] {
		t.Fatalf("expected repo_id %q in results", modRepo2.RepoID)
	}
}
