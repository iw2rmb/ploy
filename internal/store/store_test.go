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
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-roundtrip", "main", []byte(`{"type":"test"}`))

	if fx.Run.Status != types.RunStatusQueued {
		t.Fatalf("CreateRun() status=%q, want %q", fx.Run.Status, types.RunStatusQueued)
	}

	fetched, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if fetched.WaveID != fx.Wave.ID {
		t.Fatalf("GetRun().wave_id=%q, want %q", fetched.WaveID, fx.Wave.ID)
	}
	if fetched.MigID != fx.Mig.ID {
		t.Fatalf("GetRun().mig_id=%q, want %q", fetched.MigID, fx.Mig.ID)
	}
	if fetched.SpecID != fx.Spec.ID {
		t.Fatalf("GetRun().spec_id=%q, want %q", fetched.SpecID, fx.Spec.ID)
	}

	runs, err := db.ListRuns(ctx, ListRunsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListRuns() failed: %v", err)
	}
	found := false
	for _, r := range runs {
		if r.ID == fx.Run.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ListRuns() to include created run")
	}
}

func TestCreateWaveWithRuns_CreatesWaveAndRunsAtomically(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-atomic-a", "main", []byte(`{"type":"test"}`))
	repoB, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:      types.NewMigRepoID(),
		MigID:   fx.Mig.ID,
		Url:     "https://github.com/org/repo-atomic-b",
		BaseRef: "main",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo-b) failed: %v", err)
	}

	waveID := types.NewWaveID()
	wave, runs, err := db.CreateWaveWithRuns(ctx, CreateWaveWithRunsParams{
		Wave: CreateWaveParams{
			ID:        waveID,
			MigID:     fx.Mig.ID,
			SpecID:    fx.Spec.ID,
			CreatedBy: fx.Run.CreatedBy,
		},
		Runs: []CreateRunParams{
			{
				ID:              types.NewRunID(),
				WaveID:          waveID,
				MigID:           fx.Mig.ID,
				SpecID:          fx.Spec.ID,
				RepoID:          fx.MigRepo.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
			},
			{
				ID:              types.NewRunID(),
				WaveID:          waveID,
				MigID:           fx.Mig.ID,
				SpecID:          fx.Spec.ID,
				RepoID:          repoB.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateWaveWithRuns() failed: %v", err)
	}
	if wave.ID != waveID || len(runs) != 2 {
		t.Fatalf("unexpected CreateWaveWithRuns result: wave=%+v runs=%+v", wave, runs)
	}

	rollbackWaveID := types.NewWaveID()
	rollbackRunID := types.NewRunID()
	_, _, err = db.CreateWaveWithRuns(ctx, CreateWaveWithRunsParams{
		Wave: CreateWaveParams{
			ID:        rollbackWaveID,
			MigID:     fx.Mig.ID,
			SpecID:    fx.Spec.ID,
			CreatedBy: fx.Run.CreatedBy,
		},
		Runs: []CreateRunParams{
			{
				ID:              rollbackRunID,
				WaveID:          rollbackWaveID,
				MigID:           fx.Mig.ID,
				SpecID:          fx.Spec.ID,
				RepoID:          "missing1",
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
			},
		},
	})
	if err == nil {
		t.Fatal("CreateWaveWithRuns() with invalid repo unexpectedly succeeded")
	}
	if _, err := db.GetWave(ctx, rollbackWaveID); err != pgx.ErrNoRows {
		t.Fatalf("GetWave(rollback) err = %v, want pgx.ErrNoRows", err)
	}
	if _, err := db.GetRun(ctx, rollbackRunID); err != pgx.ErrNoRows {
		t.Fatalf("GetRun(rollback) err = %v, want pgx.ErrNoRows", err)
	}
}

func TestRun_CRUDAndStateTransitions_V1(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", []byte(`{"type":"wave"}`))

	if fx.Run.Status != types.RunStatusQueued {
		t.Fatalf("CreateRun() status=%q, want %q", fx.Run.Status, types.RunStatusQueued)
	}
	if fx.Run.Attempt != 1 {
		t.Fatalf("CreateRun() attempt=%d, want 1", fx.Run.Attempt)
	}

	runs, err := db.ListRunsByWave(ctx, fx.Wave.ID)
	if err != nil {
		t.Fatalf("ListRunsByWave() failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	// Transition run: Queued -> Running -> Success.
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{
		ID:     fx.Run.ID,
		Status: types.RunStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateRunStatus() to Running failed: %v", err)
	}
	updated, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if updated.Status != types.RunStatusRunning {
		t.Fatalf("run status=%q, want %q", updated.Status, types.RunStatusRunning)
	}
	if !updated.StartedAt.Valid {
		t.Fatal("expected started_at to be set for Running run")
	}
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{
		ID:     fx.Run.ID,
		Status: types.RunStatusSuccess,
	}); err != nil {
		t.Fatalf("UpdateRunStatus() to Success failed: %v", err)
	}
	final, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if final.Status != types.RunStatusSuccess {
		t.Fatalf("run status=%q, want %q", final.Status, types.RunStatusSuccess)
	}
	if !final.FinishedAt.Valid {
		t.Fatal("expected finished_at to be set for terminal run")
	}

	// Attempt increment resets run state.
	if err := db.IncrementRunAttempt(ctx, fx.Run.ID); err != nil {
		t.Fatalf("IncrementRunAttempt() failed: %v", err)
	}
	retry, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() after increment failed: %v", err)
	}
	if retry.Attempt != 2 {
		t.Fatalf("attempt=%d, want 2", retry.Attempt)
	}
	if retry.Status != types.RunStatusQueued {
		t.Fatalf("status=%q, want %q", retry.Status, types.RunStatusQueued)
	}

	msg := "boom"
	if err := db.UpdateRunError(ctx, UpdateRunErrorParams{ID: fx.Run.ID, LastError: &msg}); err != nil {
		t.Fatalf("UpdateRunError() failed: %v", err)
	}
	got, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() after error failed: %v", err)
	}
	if got.LastError == nil || *got.LastError != msg {
		t.Fatalf("last_error=%v, want %q", got.LastError, msg)
	}

	if err := db.DeleteRun(ctx, fx.Run.ID); err != nil {
		t.Fatalf("DeleteRun() failed: %v", err)
	}
	_, err = db.GetRun(ctx, fx.Run.ID)
	if err == nil {
		t.Fatal("expected GetRun() after delete to fail")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestListRunsWithURLByWave_ReturnsRepoURLAndOrdering_V1(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", []byte(`{"type":"wave"}`))

	migRepo2ID := types.NewMigRepoID()
	migRepo2, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:      migRepo2ID,
		MigID:   fx.Mig.ID,
		Url:     "https://github.com/org/repo-b",
		BaseRef: "main",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() for repo-b failed: %v", err)
	}

	_, err = db.CreateRun(ctx, CreateRunParams{
		ID:              types.NewRunID(),
		WaveID:          fx.Wave.ID,
		MigID:           fx.Mig.ID,
		SpecID:          fx.Spec.ID,
		RepoID:          migRepo2.RepoID,
		RepoBaseRef:     migRepo2.BaseRef,
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
	})
	if err != nil {
		t.Fatalf("CreateRun() for repo-b failed: %v", err)
	}

	rows, err := db.ListRunsWithURLByWave(ctx, fx.Wave.ID)
	if err != nil {
		t.Fatalf("ListRunsWithURLByWave() failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 runs with urls, got %d", len(rows))
	}

	expectedURLByRepoID := map[types.RepoID]string{
		fx.MigRepo.RepoID: "https://github.com/org/repo-a",
		migRepo2.RepoID:   "https://github.com/org/repo-b",
	}

	seen := map[types.RepoID]bool{}
	for i, row := range rows {
		if row.WaveID != fx.Wave.ID {
			t.Fatalf("row[%d] wave_id=%q, want %q", i, row.WaveID, fx.Wave.ID)
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
	if !seen[migRepo2.RepoID] {
		t.Fatalf("expected repo_id %q in results", migRepo2.RepoID)
	}
}
