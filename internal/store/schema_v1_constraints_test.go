package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestV1Schema_MigsNameUniqueness verifies the UNIQUE constraint on migs.name.
// The migs table has a unique index on name to prevent duplicate mig names.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1Schema_MigsNameUniqueness(t *testing.T) {
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

	// Clean up any existing test migs.
	testMigIDs := []string{}
	defer func() {
		for _, migID := range testMigIDs {
			_, _ = db.Pool().Exec(ctx, "DELETE FROM migs WHERE id = $1", migID)
		}
	}()

	// Insert first mig with name "test-mig-uniqueness".
	migID1 := domaintypes.NewMigID()
	testMigIDs = append(testMigIDs, migID1.String())
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO migs (id, name, created_by, created_at)
			VALUES ($1, $2, $3, now())
		`, migID1.String(), "test-mig-uniqueness", "test-user")
	if err != nil {
		t.Fatalf("first mig insert failed: %v", err)
	}

	// Attempt to insert second mig with the same name.
	migID2 := domaintypes.NewMigID()
	testMigIDs = append(testMigIDs, migID2.String())
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO migs (id, name, created_by, created_at)
			VALUES ($1, $2, $3, now())
		`, migID2.String(), "test-mig-uniqueness", "test-user")

	// Verify that the insert was rejected due to unique constraint violation.
	if err == nil {
		t.Fatal("expected duplicate name insert to fail, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	// PostgreSQL unique violation code is 23505.
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation error code 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}
}

// TestV1Schema_MigReposUniqueness verifies the UNIQUE constraint on (mig_id, repo_id).
// The mig_repos table has UNIQUE (mig_id, repo_id) to prevent duplicate repo memberships per mig.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1Schema_MigReposUniqueness(t *testing.T) {
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

	// Create a test mig.
	migID := domaintypes.NewMigID()
	defer func() {
		_, _ = db.Pool().Exec(ctx, "DELETE FROM migs WHERE id = $1", migID.String())
	}()

	_, err = db.Pool().Exec(ctx, `
			INSERT INTO migs (id, name, created_by, created_at)
			VALUES ($1, $2, $3, now())
		`, migID.String(), "test-mig-repos-uniq-"+migID.String(), "test-user")
	if err != nil {
		t.Fatalf("mig insert failed: %v", err)
	}

	// Upsert the repos row so mig_repos can reference it.
	const testRepoURL1 = "https://github.com/test/repo1.git"
	var resolvedRepoID1 string
	if err = db.Pool().QueryRow(ctx, `
			INSERT INTO repos (id, url) VALUES ($1, $2)
			ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
			RETURNING id
		`, domaintypes.NewMigRepoID().String(), testRepoURL1).Scan(&resolvedRepoID1); err != nil {
		t.Fatalf("repos upsert failed: %v", err)
	}

	// Insert first mig_repos row.
	repoID1 := domaintypes.NewMigRepoID()
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, target_ref, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, repoID1.String(), migID.String(), resolvedRepoID1, "main", "feature")
	if err != nil {
		t.Fatalf("first mig_repos insert failed: %v", err)
	}

	// Attempt to insert second mig_repos row with the same (mig_id, repo_id).
	repoID2 := domaintypes.NewMigRepoID()
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, target_ref, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, repoID2.String(), migID.String(), resolvedRepoID1, "main", "feature-2")

	// Verify that the insert was rejected due to unique constraint violation.
	if err == nil {
		t.Fatal("expected duplicate (mig_id, repo_url) insert to fail, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation error code 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}
}

// TestV1Schema_RunReposCompositePK verifies the composite PRIMARY KEY (run_id, repo_id).
// The run_repos table has PRIMARY KEY (run_id, repo_id) to ensure one entry per repo per run.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1Schema_RunReposCompositePK(t *testing.T) {
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

	// Create a test mig, spec, mig_repo, and run.
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	runID := domaintypes.NewRunID()

	defer func() {
		_, _ = db.Pool().Exec(ctx, "DELETE FROM runs WHERE id = $1", runID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM mig_repos WHERE id = $1", repoID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id = $1", specID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM migs WHERE id = $1", migID.String())
	}()

	// Insert mig.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO migs (id, name, created_by, created_at)
			VALUES ($1, $2, $3, now())
		`, migID.String(), "test-run-repos-pk-"+migID.String(), "test-user")
	if err != nil {
		t.Fatalf("mig insert failed: %v", err)
	}

	// Insert spec.
	specJSON, _ := json.Marshal(map[string]interface{}{"steps": []string{"test"}})
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO specs (id, name, spec, created_by, created_at)
			VALUES ($1, $2, $3, $4, now())
		`, specID.String(), "test-spec", specJSON, "test-user")
	if err != nil {
		t.Fatalf("spec insert failed: %v", err)
	}

	// Upsert the repos row so mig_repos can reference it.
	const testRepoURLPK = "https://github.com/test/repo-pk.git"
	var resolvedRepoIDPK string
	if err = db.Pool().QueryRow(ctx, `
			INSERT INTO repos (id, url) VALUES ($1, $2)
			ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
			RETURNING id
		`, domaintypes.NewMigRepoID().String(), testRepoURLPK).Scan(&resolvedRepoIDPK); err != nil {
		t.Fatalf("repos upsert failed: %v", err)
	}

	// Insert mig_repo.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, target_ref, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, repoID.String(), migID.String(), resolvedRepoIDPK, "main", "feature")
	if err != nil {
		t.Fatalf("mig_repos insert failed: %v", err)
	}

	// Insert run.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO runs (id, mig_id, spec_id, created_by, status, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, runID.String(), migID.String(), specID.String(), "test-user", "Started")
	if err != nil {
		t.Fatalf("run insert failed: %v", err)
	}

	// Insert first run_repos row.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO run_repos (mig_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
		`, migID.String(), runID.String(), resolvedRepoIDPK, "main", "feature", "Queued")
	if err != nil {
		t.Fatalf("first run_repos insert failed: %v", err)
	}

	// Attempt to insert second run_repos row with the same (run_id, repo_id).
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO run_repos (mig_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
		`, migID.String(), runID.String(), resolvedRepoIDPK, "main", "feature-2", "Queued")

	// Verify that the insert was rejected due to PK violation.
	if err == nil {
		t.Fatal("expected duplicate (run_id, repo_id) insert to fail, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation error code 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}
}

// TestV1Schema_JobsUniqueness verifies the UNIQUE constraint on (run_id, repo_id, attempt, name).
// The jobs table has UNIQUE (run_id, repo_id, attempt, name) to prevent duplicate jobs per repo attempt.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1Schema_JobsUniqueness(t *testing.T) {
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

	// Create a test mig, spec, mig_repo, run, and run_repo.
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	runID := domaintypes.NewRunID()
	jobID1 := domaintypes.NewJobID()
	jobID2 := domaintypes.NewJobID()

	defer func() {
		_, _ = db.Pool().Exec(ctx, "DELETE FROM jobs WHERE id = $1", jobID1.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM jobs WHERE id = $1", jobID2.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM run_repos WHERE run_id = $1 AND repo_id = $2", runID.String(), repoID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM runs WHERE id = $1", runID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM mig_repos WHERE id = $1", repoID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id = $1", specID.String())
		_, _ = db.Pool().Exec(ctx, "DELETE FROM migs WHERE id = $1", migID.String())
	}()

	// Insert mig.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO migs (id, name, created_by, created_at)
			VALUES ($1, $2, $3, now())
		`, migID.String(), "test-jobs-uniq-"+migID.String(), "test-user")
	if err != nil {
		t.Fatalf("mig insert failed: %v", err)
	}

	// Insert spec.
	specJSON, _ := json.Marshal(map[string]interface{}{"steps": []string{"test"}})
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO specs (id, name, spec, created_by, created_at)
			VALUES ($1, $2, $3, $4, now())
		`, specID.String(), "test-spec", specJSON, "test-user")
	if err != nil {
		t.Fatalf("spec insert failed: %v", err)
	}

	// Upsert the repos row so mig_repos can reference it.
	const testRepoURLJobs = "https://github.com/test/repo-jobs.git"
	var resolvedRepoIDJobs string
	if err = db.Pool().QueryRow(ctx, `
			INSERT INTO repos (id, url) VALUES ($1, $2)
			ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
			RETURNING id
		`, domaintypes.NewMigRepoID().String(), testRepoURLJobs).Scan(&resolvedRepoIDJobs); err != nil {
		t.Fatalf("repos upsert failed: %v", err)
	}

	// Insert mig_repo.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, target_ref, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, repoID.String(), migID.String(), resolvedRepoIDJobs, "main", "feature")
	if err != nil {
		t.Fatalf("mig_repos insert failed: %v", err)
	}

	// Insert run.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO runs (id, mig_id, spec_id, created_by, status, created_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, runID.String(), migID.String(), specID.String(), "test-user", "Started")
	if err != nil {
		t.Fatalf("run insert failed: %v", err)
	}

	// Insert run_repos.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO run_repos (mig_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
		`, migID.String(), runID.String(), resolvedRepoIDJobs, "main", "feature", "Queued")
	if err != nil {
		t.Fatalf("run_repos insert failed: %v", err)
	}

	// Insert first job.
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, next_id, job_type, job_image)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, jobID1.String(), runID.String(), resolvedRepoIDJobs, "main", 1, "test-job", "Created", nil, "mig", "test-image")
	if err != nil {
		t.Fatalf("first job insert failed: %v", err)
	}

	// Attempt to insert second job with the same (run_id, repo_id, attempt, name).
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, next_id, job_type, job_image)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, jobID2.String(), runID.String(), resolvedRepoIDJobs, "main", 1, "test-job", "Created", nil, "mig", "test-image")

	// Verify that the insert was rejected due to unique constraint violation.
	if err == nil {
		t.Fatal("expected duplicate (run_id, repo_id, attempt, name) insert to fail, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation error code 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}

	// Verify that a job with a different name can be inserted (same run_id, repo_id, attempt).
	jobID3 := domaintypes.NewJobID()
	defer func() {
		_, _ = db.Pool().Exec(ctx, "DELETE FROM jobs WHERE id = $1", jobID3.String())
	}()

	_, err = db.Pool().Exec(ctx, `
			INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, next_id, job_type, job_image)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, jobID3.String(), runID.String(), resolvedRepoIDJobs, "main", 1, "test-job-2", "Created", nil, "mig", "test-image")
	if err != nil {
		t.Fatalf("job insert with different name should succeed, but failed: %v", err)
	}
}

// assertPgError checks if the error is a pgconn.PgError and assigns it to the target.
// Returns true if the error is a PgError, false otherwise.
func assertPgError(err error, target **pgconn.PgError) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		*target = pgErr
		return true
	}
	return false
}
