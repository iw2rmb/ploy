package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
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

	if fx.Run.Status != types.RunStatusRunning {
		t.Fatalf("CreateRun() status=%q, want %q", fx.Run.Status, types.RunStatusRunning)
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

	hash := gitlabtoken.Hash("glpat-store-secret")
	stats, err := gitlabtoken.RunStatsWithMarker(hash)
	if err != nil {
		t.Fatalf("build token marker stats: %v", err)
	}
	runWithStats, err := insertRun(ctx, db.(*PgStore).Queries, CreateWaveParams{
		ID: fx.Wave.ID, MigID: fx.Mig.ID, SpecID: fx.Spec.ID, CreatedBy: fx.Run.CreatedBy,
	}, RunPlan{
		ID:              types.NewRunID(),
		RepoID:          fx.MigRepo.RepoID,
		RepoBaseRef:     "main",
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
		Stats:           stats,
	})
	if err != nil {
		t.Fatalf("CreateRun(with stats) failed: %v", err)
	}
	fetchedWithStats, err := db.GetRun(ctx, runWithStats.ID)
	if err != nil {
		t.Fatalf("GetRun(with stats) failed: %v", err)
	}
	if got := gitlabtoken.HashFromRunStats(fetchedWithStats.Stats); got != hash {
		t.Fatalf("stats token hash marker = %q, want %q", got, hash)
	}
}

func TestRunStatusHasNoDefault(t *testing.T) {
	ctx, db := newTestStore(t)

	var defaultExpression *string
	err := db.Pool().QueryRow(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = 'ploy' AND table_name = 'runs' AND column_name = 'status'
	`).Scan(&defaultExpression)
	if err != nil {
		t.Fatalf("read runs.status default: %v", err)
	}
	if defaultExpression != nil {
		t.Fatalf("runs.status default=%q, want no default", *defaultExpression)
	}
}

func TestListRunsWithMetadataRepoURLFiltersBeforePagination(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-list-filter", "main", []byte(`{"steps":[{"image":"test-image"}]}`))
	alice := "alice"
	bob := "bob"

	ownerRun, err := insertRun(ctx, db.(*PgStore).Queries, CreateWaveParams{
		ID: fx.Wave.ID, MigID: fx.Mig.ID, SpecID: fx.Spec.ID, CreatedBy: &alice,
	}, RunPlan{
		ID:              types.NewRunID(),
		RepoID:          fx.MigRepo.RepoID,
		RepoBaseRef:     "main",
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
	})
	if err != nil {
		t.Fatalf("CreateRun(owner) failed: %v", err)
	}
	otherRun, err := insertRun(ctx, db.(*PgStore).Queries, CreateWaveParams{
		ID: fx.Wave.ID, MigID: fx.Mig.ID, SpecID: fx.Spec.ID, CreatedBy: &bob,
	}, RunPlan{
		ID:              types.NewRunID(),
		RepoID:          fx.MigRepo.RepoID,
		RepoBaseRef:     "main",
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
	})
	if err != nil {
		t.Fatalf("CreateRun(other) failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE runs SET created_at = now() - interval '3 seconds' WHERE id = $1", fx.Run.ID); err != nil {
		t.Fatalf("set fixture created_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE runs SET created_at = now() - interval '2 seconds' WHERE id = $1", ownerRun.ID); err != nil {
		t.Fatalf("set owner created_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE runs SET created_at = now() - interval '1 second' WHERE id = $1", otherRun.ID); err != nil {
		t.Fatalf("set other created_at failed: %v", err)
	}

	tests := []struct {
		name      string
		params    ListRunsWithMetadataParams
		wantRunID types.RunID
	}{
		{
			name: "repo and created_by filter before limit",
			params: ListRunsWithMetadataParams{
				CreatedBy:  alice,
				RepoUrl:    "https://github.com/org/repo-list-filter",
				LimitRows:  1,
				OffsetRows: 0,
			},
			wantRunID: ownerRun.ID,
		},
		{
			name: "all bypasses created_by filter",
			params: ListRunsWithMetadataParams{
				AllRuns:    true,
				CreatedBy:  alice,
				RepoUrl:    "https://github.com/org/repo-list-filter",
				LimitRows:  1,
				OffsetRows: 0,
			},
			wantRunID: otherRun.ID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := db.ListRunsWithMetadata(ctx, tt.params)
			if err != nil {
				t.Fatalf("ListRunsWithMetadata() failed: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("rows length = %d, want 1", len(rows))
			}
			if rows[0].ID != tt.wantRunID {
				t.Fatalf("run id = %q, want %q", rows[0].ID, tt.wantRunID)
			}
		})
	}
}

func TestCreateWaveWithRuns_CreatesWaveAndRunsAtomically(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-atomic-a", "main", []byte(`{"type":"test"}`))
	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRun(fixture): %v", err)
	}
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
	wave, materialized, err := db.CreateWaveWithRuns(ctx, CreateWaveWithRunsParams{
		Wave: CreateWaveParams{
			ID:        waveID,
			MigID:     fx.Mig.ID,
			SpecID:    fx.Spec.ID,
			CreatedBy: fx.Run.CreatedBy,
		},
		Runs: []RunPlan{
			{
				ID:              types.NewRunID(),
				RepoID:          fx.MigRepo.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
				Jobs:            plannedJobsForStoreTest("a-head", "a-tail"),
			},
			{
				ID:              types.NewRunID(),
				RepoID:          repoB.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
				Jobs:            plannedJobsForStoreTest("b-head", "b-tail"),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateWaveWithRuns() failed: %v", err)
	}
	if wave.ID != waveID || len(materialized) != 2 {
		t.Fatalf("unexpected CreateWaveWithRuns result: wave=%+v runs=%+v", wave, materialized)
	}
	for _, result := range materialized {
		run := result.Run
		if run.Status != types.RunStatusRunning || !run.StartedAt.Valid {
			t.Fatalf("run %s was not started atomically: status=%s started_at=%v", run.ID, run.Status, run.StartedAt.Valid)
		}
		jobs, err := db.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: run.ID, Attempt: run.Attempt})
		if err != nil || len(jobs) != 2 {
			t.Fatalf("run %s jobs=%d err=%v, want 2 jobs", run.ID, len(jobs), err)
		}
		if len(result.Jobs) != len(jobs) {
			t.Fatalf("run %s returned jobs=%d, want %d", run.ID, len(result.Jobs), len(jobs))
		}
		jobsByID := make(map[types.JobID]Job, len(jobs))
		var head Job
		for _, job := range jobs {
			jobsByID[job.ID] = job
			if job.Status == types.JobStatusQueued {
				if !head.ID.IsZero() {
					t.Fatalf("run %s has more than one queued job", run.ID)
				}
				head = job
			}
		}
		if head.ID.IsZero() || head.NextID == nil {
			t.Fatalf("run %s has no queued chain head", run.ID)
		}
		tail, ok := jobsByID[*head.NextID]
		if !ok || tail.Status != types.JobStatusCreated || tail.NextID != nil {
			t.Fatalf("run %s tail was not derived from plan order: %+v", run.ID, tail)
		}
		if head.RepoShaIn != testSHA {
			t.Fatalf("run %s head repo_sha_in=%q, want %q", run.ID, head.RepoShaIn, testSHA)
		}
	}

	rollbackWaveID := types.NewWaveID()
	for _, result := range materialized {
		if err := db.CancelRun(ctx, result.Run.ID); err != nil {
			t.Fatalf("CancelRun(%s): %v", result.Run.ID, err)
		}
	}
	materializedBeforeFailureRunID := types.NewRunID()
	rollbackRunID := types.NewRunID()
	_, _, err = db.CreateWaveWithRuns(ctx, CreateWaveWithRunsParams{
		Wave: CreateWaveParams{
			ID:        rollbackWaveID,
			MigID:     fx.Mig.ID,
			SpecID:    fx.Spec.ID,
			CreatedBy: fx.Run.CreatedBy,
		},
		Runs: []RunPlan{
			{
				ID:              materializedBeforeFailureRunID,
				RepoID:          fx.MigRepo.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
				Jobs:            plannedJobsForStoreTest(),
			},
			{
				ID:              rollbackRunID,
				RepoID:          repoB.RepoID,
				RepoBaseRef:     "main",
				SourceCommitSha: testSHA,
				RepoSha0:        testSHA,
			},
		},
	})
	if err == nil {
		t.Fatal("CreateWaveWithRuns() without jobs unexpectedly succeeded")
	}
	if _, err := db.GetWave(ctx, rollbackWaveID); err != pgx.ErrNoRows {
		t.Fatalf("GetWave(rollback) err = %v, want pgx.ErrNoRows", err)
	}
	for _, runID := range []types.RunID{materializedBeforeFailureRunID, rollbackRunID} {
		if _, err := db.GetRun(ctx, runID); err != pgx.ErrNoRows {
			t.Fatalf("GetRun(%s) err = %v, want pgx.ErrNoRows", runID, err)
		}
	}
	if jobs, err := db.ListJobsByRun(ctx, materializedBeforeFailureRunID); err != nil || len(jobs) != 0 {
		t.Fatalf("ListJobsByRun(%s) jobs=%d err=%v, want no jobs", materializedBeforeFailureRunID, len(jobs), err)
	}
}

func TestCreateWaveWithRuns_AssignsDistinctJobIDsPerRun(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-job-rollback-a", "main", []byte(`{"type":"test"}`))
	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRun(fixture): %v", err)
	}
	repoB, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:      types.NewMigRepoID(),
		MigID:   fx.Mig.ID,
		Url:     "https://github.com/org/repo-job-rollback-b",
		BaseRef: "main",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo-b) failed: %v", err)
	}

	waveID := types.NewWaveID()
	runAID := types.NewRunID()
	runBID := types.NewRunID()
	sharedPlan := plannedJobsForStoreTest("head", "tail")
	_, materialized, err := db.CreateWaveWithRuns(ctx, CreateWaveWithRunsParams{
		Wave: CreateWaveParams{ID: waveID, MigID: fx.Mig.ID, SpecID: fx.Spec.ID},
		Runs: []RunPlan{
			{
				ID: runAID, RepoID: fx.MigRepo.RepoID, RepoBaseRef: "main", SourceCommitSha: testSHA, RepoSha0: testSHA, Jobs: sharedPlan,
			},
			{
				ID: runBID, RepoID: repoB.RepoID, RepoBaseRef: "main", SourceCommitSha: testSHA, RepoSha0: testSHA, Jobs: sharedPlan,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateWaveWithRuns() failed: %v", err)
	}
	if len(materialized) != 2 || len(materialized[0].Jobs) != 2 || len(materialized[1].Jobs) != 2 {
		t.Fatalf("unexpected materialization shape: %+v", materialized)
	}
	for i := range materialized[0].Jobs {
		if materialized[0].Jobs[i].ID == materialized[1].Jobs[i].ID {
			t.Fatalf("job %d reused id %s across runs", i, materialized[0].Jobs[i].ID)
		}
	}
}

func TestRun_CRUDAndStateTransitions_V1(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-a", "main", []byte(`{"type":"wave"}`))

	if fx.Run.Status != types.RunStatusRunning {
		t.Fatalf("CreateRun() status=%q, want %q", fx.Run.Status, types.RunStatusRunning)
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

	if !fx.Run.StartedAt.Valid {
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
	if err := incrementRunAttempt(ctx, db.(*PgStore).Queries, fx.Run.ID, nil); err != nil {
		t.Fatalf("IncrementRunAttempt() failed: %v", err)
	}
	retry, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() after increment failed: %v", err)
	}
	if retry.Attempt != 2 {
		t.Fatalf("attempt=%d, want 2", retry.Attempt)
	}
	if retry.Status != types.RunStatusRunning {
		t.Fatalf("status=%q, want %q", retry.Status, types.RunStatusRunning)
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

	_, err = insertRun(ctx, db.(*PgStore).Queries, CreateWaveParams{
		ID: fx.Wave.ID, MigID: fx.Mig.ID, SpecID: fx.Spec.ID, CreatedBy: fx.Run.CreatedBy,
	}, RunPlan{
		ID:              types.NewRunID(),
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
