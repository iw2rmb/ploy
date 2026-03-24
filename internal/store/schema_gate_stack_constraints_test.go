package store

import (
	"context"
	"os"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSchema_GateProfilesUniqueByRepoShaStack(t *testing.T) {
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
	cleanTestTables(t, ctx, db)

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	repoID := domaintypes.NewMigRepoID().String()
	const repoURL = "https://github.com/test/gate-profile-uniq.git"
	if _, err := tx.Exec(ctx, `
		INSERT INTO repos (id, url)
		VALUES ($1, $2)
	`, repoID, repoURL); err != nil {
		t.Fatalf("insert repos row: %v", err)
	}

	var stackID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO stacks (lang, release, tool, image)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lang, release, tool) DO UPDATE SET image = EXCLUDED.image
		RETURNING id
	`, "java", "17", "maven", "example.com/maven:17").Scan(&stackID); err != nil {
		t.Fatalf("insert stacks row: %v", err)
	}

	const repoSHA = "0123456789abcdef0123456789abcdef01234567"
	if _, err := tx.Exec(ctx, `
		INSERT INTO gate_profiles (repo_id, repo_sha, repo_sha8, stack_id, url)
		VALUES ($1, $2, $3, $4, $5)
	`, repoID, repoSHA, repoSHA[:8], stackID, "garage://profiles/exact-1.yaml"); err != nil {
		t.Fatalf("insert gate_profiles row: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gate_profiles (repo_id, repo_sha, repo_sha8, stack_id, url)
		VALUES ($1, $2, $3, $4, $5)
	`, repoID, repoSHA, repoSHA[:8], stackID, "garage://profiles/exact-2.yaml")
	if err == nil {
		t.Fatal("expected duplicate (repo_id, repo_sha, stack_id) insert to fail")
	}

	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Fatalf("expected unique violation 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}
}

func TestSchema_GateProfilesForeignKeys(t *testing.T) {
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
	cleanTestTables(t, ctx, db)

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `
		INSERT INTO gate_profiles (repo_id, repo_sha, stack_id, url)
		VALUES ($1, $2, $3, $4)
	`, "missing_repo", "0123456789abcdef0123456789abcdef01234567", int64(999999), "garage://profiles/missing.yaml")
	if err == nil {
		t.Fatal("expected gate_profiles insert to fail for missing FK rows")
	}

	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23503" {
		t.Fatalf("expected foreign-key violation 23503, got %s: %s", pgErr.Code, pgErr.Message)
	}
}

func TestSchema_GatesUniqueJobAndProfileFK(t *testing.T) {
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
	cleanTestTables(t, ctx, db)

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	migRepoID := domaintypes.NewMigRepoID().String()
	runID := domaintypes.NewRunID().String()
	jobID := domaintypes.NewJobID().String()
	jobID2 := domaintypes.NewJobID().String()
	repoID := domaintypes.NewMigRepoID().String()

	if _, err := tx.Exec(ctx, `
		INSERT INTO migs (id, name, created_by)
		VALUES ($1, $2, $3)
	`, migID, "test-gates-constraints-"+migID, "test-user"); err != nil {
		t.Fatalf("insert mig: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO specs (id, name, spec, created_by)
		VALUES ($1, $2, $3::jsonb, $4)
	`, specID, "test-spec", `{}`, "test-user"); err != nil {
		t.Fatalf("insert spec: %v", err)
	}

	// Upsert into repos so mig_repos FK can reference it.
	const gatesJobRepoURL = "https://github.com/test/gates-job.git"
	var gatesJobRepoID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO repos (id, url) VALUES ($1, $2)
		ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		RETURNING id
	`, repoID, gatesJobRepoURL).Scan(&gatesJobRepoID); err != nil {
		t.Fatalf("upsert repos row for gates test: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, target_ref)
		VALUES ($1, $2, $3, $4, $5)
	`, migRepoID, migID, gatesJobRepoID, "main", "feature"); err != nil {
		t.Fatalf("insert mig_repo: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO runs (id, mig_id, spec_id, created_by, status)
		VALUES ($1, $2, $3, $4, $5)
	`, runID, migID, specID, "test-user", "Started"); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, job_type, job_image)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, jobID, runID, gatesJobRepoID, "main", 1, "pre_gate", "Created", "pre_gate", "example.com/gate:latest"); err != nil {
		t.Fatalf("insert first job: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, job_type, job_image)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, jobID2, runID, gatesJobRepoID, "main", 1, "post_gate", "Created", "post_gate", "example.com/gate:latest"); err != nil {
		t.Fatalf("insert second job: %v", err)
	}

	profilesRepoID := domaintypes.NewMigRepoID().String()
	if _, err := tx.Exec(ctx, `
		INSERT INTO repos (id, url)
		VALUES ($1, $2)
	`, profilesRepoID, "https://github.com/test/gates-profiles.git"); err != nil {
		t.Fatalf("insert repos row: %v", err)
	}

	var stackID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO stacks (lang, release, tool, image)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lang, release, tool) DO UPDATE SET image = EXCLUDED.image
		RETURNING id
	`, "java", "17", "maven", "example.com/maven:17").Scan(&stackID); err != nil {
		t.Fatalf("insert stack row: %v", err)
	}

	var profileID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO gate_profiles (repo_id, repo_sha, repo_sha8, stack_id, url)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, repoID, "0123456789abcdef0123456789abcdef01234567", "01234567", stackID, "garage://profiles/exact.yaml").Scan(&profileID); err != nil {
		t.Fatalf("insert gate_profile row: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO gates (job_id, profile_id)
		VALUES ($1, $2)
	`, jobID, profileID); err != nil {
		t.Fatalf("insert gates row: %v", err)
	}

	// Use savepoints so expected errors don't abort the outer transaction.
	var pgErr *pgconn.PgError

	if _, spErr := tx.Exec(ctx, "SAVEPOINT sp_dup"); spErr != nil {
		t.Fatalf("savepoint sp_dup: %v", spErr)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO gates (job_id, profile_id)
		VALUES ($1, $2)
	`, jobID, profileID)
	if err == nil {
		t.Fatal("expected duplicate gates.job_id insert to fail")
	}
	if _, spErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp_dup"); spErr != nil {
		t.Fatalf("rollback to sp_dup: %v", spErr)
	}
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Fatalf("expected unique violation 23505, got %s: %s", pgErr.Code, pgErr.Message)
	}

	if _, spErr := tx.Exec(ctx, "SAVEPOINT sp_fk"); spErr != nil {
		t.Fatalf("savepoint sp_fk: %v", spErr)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO gates (job_id, profile_id)
		VALUES ($1, $2)
	`, jobID2, int64(999999))
	if err == nil {
		t.Fatal("expected gates insert with missing profile to fail")
	}
	if _, spErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp_fk"); spErr != nil {
		t.Fatalf("rollback to sp_fk: %v", spErr)
	}
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23503" {
		t.Fatalf("expected foreign-key violation 23503, got %s: %s", pgErr.Code, pgErr.Message)
	}
}
