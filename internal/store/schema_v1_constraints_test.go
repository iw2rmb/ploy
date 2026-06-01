package store

import (
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestV1Schema_MigsNameUniqueness verifies the UNIQUE constraint on migs.name.
// The migs table has a unique index on name to prevent duplicate mig names.
func TestV1Schema_MigsNameUniqueness(t *testing.T) {
	ctx, db := newTestStore(t)

	// Insert first mig with name "test-mig-uniqueness".
	migID1 := domaintypes.NewMigID()
	_, err := db.Pool().Exec(ctx, `
				INSERT INTO migs (id, name, created_by, created_at)
				VALUES ($1, $2, $3, now())
			`, migID1.String(), "test-mig-uniqueness", "test-user")
	if err != nil {
		t.Fatalf("first mig insert failed: %v", err)
	}

	// Attempt to insert second mig with the same name.
	migID2 := domaintypes.NewMigID()
	_, err = db.Pool().Exec(ctx, `
				INSERT INTO migs (id, name, created_by, created_at)
				VALUES ($1, $2, $3, now())
		`, migID2.String(), "test-mig-uniqueness", "test-user")

	if err == nil {
		t.Fatal("expected duplicate name insert to fail, but it succeeded")
	}
	assertUniqueViolation(t, err)
}

// TestV1Schema_MigReposUniqueness verifies the UNIQUE constraint on (mig_id, repo_id).
// The mig_repos table has UNIQUE (mig_id, repo_id) to prevent duplicate repo memberships per mig.
func TestV1Schema_MigReposUniqueness(t *testing.T) {
	ctx, db := newTestStore(t)

	// Create a test mig.
	migID := domaintypes.NewMigID()
	_, err := db.Pool().Exec(ctx, `
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
			`, domaintypes.NewRepoID().String(), testRepoURL1).Scan(&resolvedRepoID1); err != nil {
		t.Fatalf("repos upsert failed: %v", err)
	}

	// Insert first mig_repos row.
	repoID1 := domaintypes.NewMigRepoID()
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, created_at)
			VALUES ($1, $2, $3, $4, now())
		`, repoID1.String(), migID.String(), resolvedRepoID1, "main")
	if err != nil {
		t.Fatalf("first mig_repos insert failed: %v", err)
	}

	// Attempt to insert second mig_repos row with the same (mig_id, repo_id).
	repoID2 := domaintypes.NewMigRepoID()
	_, err = db.Pool().Exec(ctx, `
			INSERT INTO mig_repos (id, mig_id, repo_id, base_ref, created_at)
			VALUES ($1, $2, $3, $4, now())
		`, repoID2.String(), migID.String(), resolvedRepoID1, "main")

	if err == nil {
		t.Fatal("expected duplicate (mig_id, repo_url) insert to fail, but it succeeded")
	}
	assertUniqueViolation(t, err)
}

// TestV1Schema_RunsPrimaryKey verifies the current runs.id primary key.
func TestV1Schema_RunsPrimaryKey(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/repo-pk.git", "main", []byte(`{"steps":["test"]}`))

	_, err := db.Pool().Exec(ctx, `
			INSERT INTO runs (
				id, wave_id, mig_id, spec_id, repo_id, repo_base_ref,
				source_commit_sha, repo_sha0, status, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'Queued', now())
		`, fx.Run.ID.String(), fx.Wave.ID.String(), fx.Mig.ID.String(), fx.Spec.ID.String(),
		fx.MigRepo.RepoID.String(), fx.Run.RepoBaseRef, testSHA, testSHA)

	if err == nil {
		t.Fatal("expected duplicate run id insert to fail, but it succeeded")
	}
	assertUniqueViolation(t, err)
}

// TestV1Schema_JobsPrimaryKey verifies the current jobs.id primary key.
func TestV1Schema_JobsPrimaryKey(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/repo-jobs.git", "main", []byte(`{"steps":["test"]}`))
	jobID := domaintypes.NewJobID()

	_, err := db.Pool().Exec(ctx, `
			INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, next_id, job_type, job_image)
			VALUES ($1, $2, $3, $4, $5, $6, 'Created', NULL, 'mig', 'test-image')
		`, jobID.String(), fx.Run.ID.String(), fx.MigRepo.RepoID.String(), fx.Run.RepoBaseRef, fx.Run.Attempt, "test-job")
	if err != nil {
		t.Fatalf("first job insert failed: %v", err)
	}

	_, err = db.Pool().Exec(ctx, `
			INSERT INTO jobs (id, run_id, repo_id, repo_base_ref, attempt, name, status, next_id, job_type, job_image)
			VALUES ($1, $2, $3, $4, $5, $6, 'Created', NULL, 'mig', 'test-image')
		`, jobID.String(), fx.Run.ID.String(), fx.MigRepo.RepoID.String(), fx.Run.RepoBaseRef, fx.Run.Attempt, "test-job-duplicate")

	if err == nil {
		t.Fatal("expected duplicate job id insert to fail, but it succeeded")
	}
	assertUniqueViolation(t, err)
}

func assertUniqueViolation(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !assertPgError(err, &pgErr) {
		t.Fatalf("expected pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Errorf("expected unique violation error code 23505, got %s: %s", pgErr.Code, pgErr.Message)
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
