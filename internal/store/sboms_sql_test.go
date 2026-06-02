package store

import (
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSBOMInsertQuery_UsesConflictNoop(t *testing.T) {
	t.Parallel()

	want := "ON CONFLICT (job_id, repo_id, lib, ver) DO NOTHING"
	if !strings.Contains(normalizeWhitespace(upsertSBOMRow), normalizeWhitespace(want)) {
		t.Fatalf("UpsertSBOMRow must keep idempotent insert semantics; want %q in SQL:\n%s", want, upsertSBOMRow)
	}
}

func TestSBOMConstraint_PrimaryKeyDefinedInSchema(t *testing.T) {
	t.Parallel()

	schema := normalizeWhitespace(schemaSQL)
	wantTable := normalizeWhitespace("CREATE TABLE IF NOT EXISTS sboms")
	wantPK := normalizeWhitespace("PRIMARY KEY (job_id, repo_id, lib, ver)")
	if !strings.Contains(schema, wantTable) {
		t.Fatalf("schema missing sboms table definition")
	}
	if !strings.Contains(schema, wantPK) {
		t.Fatalf("schema missing sboms primary key definition")
	}

	start := strings.Index(schema, normalizeWhitespace("CREATE TABLE IF NOT EXISTS sboms ("))
	if start == -1 {
		t.Fatalf("schema missing sboms table start")
	}
	tableSection := schema[start:]
	end := strings.Index(tableSection, ");")
	if end == -1 {
		t.Fatalf("schema missing sboms table end")
	}
	if strings.Contains(tableSection[:end], "created_at") {
		t.Fatalf("sboms table must not define created_at; time attribution must come from jobs.created_at via join")
	}
}

func TestListRunSBOMRowsByJobType_CurrentAttemptScope(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-sbom", "main", []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`))
	oldPre := createSBOMJobForStoreTest(t, ctx, db, fx.Run, types.JobTypePreGate, 1, "old-pre")
	upsertSBOMForStoreTest(t, ctx, db, oldPre, fx.Run.RepoID, "old-lib", "0.1.0")

	if err := db.IncrementRunAttempt(ctx, fx.Run.ID); err != nil {
		t.Fatalf("IncrementRunAttempt() failed: %v", err)
	}
	run, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}

	currentPre := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypePreGate, run.Attempt, "current-pre")
	currentPost := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypePostGate, run.Attempt, "current-post")
	currentMig := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypeMig, run.Attempt, "current-mig")
	upsertSBOMForStoreTest(t, ctx, db, currentPre, run.RepoID, "alpha", "1.0.0")
	upsertSBOMForStoreTest(t, ctx, db, currentPre, run.RepoID, "beta", "2.0.0")
	upsertSBOMForStoreTest(t, ctx, db, currentPost, run.RepoID, "post-only", "1.0.0")
	upsertSBOMForStoreTest(t, ctx, db, currentMig, run.RepoID, "mig-only", "1.0.0")

	other := newV1Fixture(t, ctx, db, "https://github.com/org/repo-sbom-other", "main", []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`))
	otherPre := createSBOMJobForStoreTest(t, ctx, db, other.Run, types.JobTypePreGate, other.Run.Attempt, "other-pre")
	upsertSBOMForStoreTest(t, ctx, db, otherPre, other.Run.RepoID, "other-lib", "9.0.0")

	rows, err := db.ListRunSBOMRowsByJobType(ctx, ListRunSBOMRowsByJobTypeParams{
		RunID:   run.ID,
		JobType: types.JobTypePreGate,
	})
	if err != nil {
		t.Fatalf("ListRunSBOMRowsByJobType() failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len=%d, want 2: %+v", len(rows), rows)
	}
	if rows[0].Lib != "alpha" || rows[0].Ver != "1.0.0" || rows[1].Lib != "beta" || rows[1].Ver != "2.0.0" {
		t.Fatalf("rows=%+v, want alpha/beta current pre rows", rows)
	}
}

func createSBOMJobForStoreTest(t *testing.T, ctx context.Context, db Store, run Run, jobType types.JobType, attempt int32, name string) Job {
	t.Helper()
	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      run.RepoID,
		RepoBaseRef: run.RepoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      types.JobStatusSuccess,
		JobType:     jobType,
		JobImage:    "test-image",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s) failed: %v", name, err)
	}
	return job
}

func upsertSBOMForStoreTest(t *testing.T, ctx context.Context, db Store, job Job, repoID types.RepoID, lib, ver string) {
	t.Helper()
	if err := db.UpsertSBOMRow(ctx, UpsertSBOMRowParams{
		JobID:  job.ID,
		RepoID: repoID,
		Lib:    lib,
		Ver:    ver,
	}); err != nil {
		t.Fatalf("UpsertSBOMRow(%s %s) failed: %v", lib, ver, err)
	}
}
