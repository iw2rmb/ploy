package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHasSBOMEvidenceForStack_UsesLatestSuccessfulSBOMFromSuccessfulRuns(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-has", "main", "feature", []byte(`{"type":"sbom-compat-has"}`))
	setRunRepoStatusForSBOMCompatTest(t, ctx, db, fx, types.RunRepoStatusSuccess)

	mavenStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "maven", "example.com/java:17-maven")
	mavenProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, mavenStackID, "1111111111111111111111111111111111111111")
	// Latest successful sbom snapshot in the successful run carries evidence.
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "pre-gate", types.JobTypePreGate, types.JobStatusSuccess, "alpha", "1.0.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "alpha", "1.1.0")

	// Failed run must be ignored even when sbom job succeeds.
	fxFailed := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-has-failed", "main", "feature", []byte(`{"type":"sbom-compat-has-failed"}`))
	setRunRepoStatusForSBOMCompatTest(t, ctx, db, fxFailed, types.RunRepoStatusFail)
	mavenFailedProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fxFailed.RunRepo.RepoID, mavenStackID, "2222222222222222222222222222222222222222")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fxFailed, mavenFailedProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "ignored", "0.0.1")

	has, err := db.HasSBOMEvidenceForStack(ctx, HasSBOMEvidenceForStackParams{
		Lang:    "java",
		Release: "17",
		Tool:    "maven",
	})
	if err != nil {
		t.Fatalf("HasSBOMEvidenceForStack(maven) failed: %v", err)
	}
	if !has {
		t.Fatal("HasSBOMEvidenceForStack(maven)=false, want true with successful sbom snapshot mapped to the stack")
	}

	has, err = db.HasSBOMEvidenceForStack(ctx, HasSBOMEvidenceForStackParams{
		Lang:    "java",
		Release: "21",
		Tool:    "maven",
	})
	if err != nil {
		t.Fatalf("HasSBOMEvidenceForStack(java-21) failed: %v", err)
	}
	if has {
		t.Fatal("HasSBOMEvidenceForStack(java-21)=true, want false for a release without evidence")
	}
}

func TestListSBOMCompatRows_UsesLatestSuccessfulSBOMFromSuccessfulRuns(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-list", "main", "feature", []byte(`{"type":"sbom-compat-list"}`))
	setRunRepoStatusForSBOMCompatTest(t, ctx, db, fx, types.RunRepoStatusSuccess)

	mavenStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "maven", "example.com/java:17-maven")

	mavenProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, mavenStackID, "3333333333333333333333333333333333333333")
	// Earlier sbom snapshot in the same successful run.
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "pre-gate", types.JobTypePreGate, types.JobStatusSuccess, "alpha", "1.0.0")
	// Latest sbom snapshot in the same successful run (must win).
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "alpha", "1.4.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "beta", "2.1.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "org:lib", "3.1.0")

	// Another successful run contributes its own latest sbom snapshot.
	fx2 := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-list-2", "main", "feature", []byte(`{"type":"sbom-compat-list-2"}`))
	setRunRepoStatusForSBOMCompatTest(t, ctx, db, fx2, types.RunRepoStatusSuccess)
	mavenProfileID2 := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx2.RunRepo.RepoID, mavenStackID, "4444444444444444444444444444444444444444")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx2, mavenProfileID2, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "alpha", "1.3.0")

	// Failed run must be ignored.
	fxFailed := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-list-failed", "main", "feature", []byte(`{"type":"sbom-compat-list-failed"}`))
	setRunRepoStatusForSBOMCompatTest(t, ctx, db, fxFailed, types.RunRepoStatusFail)
	mavenFailedProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fxFailed.RunRepo.RepoID, mavenStackID, "5555555555555555555555555555555555555555")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fxFailed, mavenFailedProfileID, "post-gate", types.JobTypePostGate, types.JobStatusSuccess, "alpha", "0.0.1")

	rows, err := db.ListSBOMCompatRows(ctx, ListSBOMCompatRowsParams{
		Lang:    "java",
		Release: "17",
		Tool:    "maven",
		Libs:    []string{"alpha", "beta", "org:lib", "missing"},
	})
	if err != nil {
		t.Fatalf("ListSBOMCompatRows() failed: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("ListSBOMCompatRows() returned %d rows, want 4", len(rows))
	}
	want := []ListSBOMCompatRowsRow{
		{Lib: "alpha", Ver: "1.3.0"},
		{Lib: "alpha", Ver: "1.4.0"},
		{Lib: "beta", Ver: "2.1.0"},
		{Lib: "org:lib", Ver: "3.1.0"},
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row[%d]=%+v, want %+v", i, rows[i], want[i])
		}
	}

	emptyRows, err := db.ListSBOMCompatRows(ctx, ListSBOMCompatRowsParams{
		Lang:    "java",
		Release: "17",
		Tool:    "maven",
		Libs:    nil,
	})
	if err != nil {
		t.Fatalf("ListSBOMCompatRows(nil libs) failed: %v", err)
	}
	if len(emptyRows) != 0 {
		t.Fatalf("ListSBOMCompatRows(nil libs) returned %d rows, want 0", len(emptyRows))
	}
}

func setRunRepoStatusForSBOMCompatTest(t *testing.T, ctx context.Context, db Store, fx v1Fixture, status types.RunRepoStatus) {
	t.Helper()
	if err := db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
		RunID:  fx.Run.ID,
		RepoID: fx.RunRepo.RepoID,
		Status: status,
	}); err != nil {
		t.Fatalf("UpdateRunRepoStatus(%s, %s): %v", fx.Run.ID, status, err)
	}
}

func upsertStackForSBOMCompatTest(t *testing.T, ctx context.Context, db Store, lang, release, tool, image string) int64 {
	t.Helper()

	var toolParam interface{}
	if tool == "" {
		toolParam = nil
	} else {
		toolParam = tool
	}

	var stackID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO stacks (lang, release, tool, image)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lang, release, tool) DO UPDATE SET image = EXCLUDED.image
		RETURNING id
	`, lang, release, toolParam, image).Scan(&stackID)
	if err != nil {
		t.Fatalf("upsert stack (%s,%s,%s): %v", lang, release, tool, err)
	}
	return stackID
}

func upsertGateProfileForSBOMCompatTest(t *testing.T, ctx context.Context, db Store, repoID types.RepoID, stackID int64, repoSHA string) int64 {
	t.Helper()

	row, err := db.UpsertExactGateProfile(ctx, UpsertExactGateProfileParams{
		RepoID:  repoID.String(),
		RepoSha: repoSHA,
		StackID: stackID,
		Url:     "garage://profiles/" + repoSHA + ".yaml",
	})
	if err != nil {
		t.Fatalf("UpsertExactGateProfile(%s): %v", repoSHA, err)
	}
	return row.ID
}

func createGateJobAndSBOMRowForSBOMCompatTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	fx v1Fixture,
	profileID int64,
	cycleName string,
	gateType types.JobType,
	gateStatus types.JobStatus,
	lib, ver string,
) {
	t.Helper()

	gateJob, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fx.Run.ID,
		RepoID:      fx.RunRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     1,
		Name:        cycleName,
		Status:      gateStatus,
		JobType:     gateType,
		JobImage:    "example.com/gate:latest",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(gate %s): %v", cycleName, err)
	}
	if err := db.UpsertGateJobProfileLink(ctx, UpsertGateJobProfileLinkParams{
		JobID:     gateJob.ID.String(),
		ProfileID: profileID,
	}); err != nil {
		t.Fatalf("UpsertGateJobProfileLink(gate=%s): %v", gateJob.ID, err)
	}
	if err := db.UpsertSBOMRow(ctx, UpsertSBOMRowParams{
		JobID:  gateJob.ID,
		RepoID: fx.RunRepo.RepoID,
		Lib:    lib,
		Ver:    ver,
	}); err != nil {
		t.Fatalf("UpsertSBOMRow(gate job=%s, lib=%s, ver=%s): %v", gateJob.ID, lib, ver, err)
	}
}
