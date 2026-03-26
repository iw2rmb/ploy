package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHasSBOMEvidenceForStack_FiltersByGateStatusTypeAndStack(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-has", "main", "feature", []byte(`{"type":"sbom-compat-has"}`))

	mavenStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "maven", "example.com/java:17-maven")
	gradleStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "gradle", "example.com/java:17-gradle")

	mavenProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, mavenStackID, "1111111111111111111111111111111111111111")
	gradleProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, gradleStackID, "2222222222222222222222222222222222222222")

	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePreGate, types.JobStatusFail, "alpha", "1.0.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypeMod, types.JobStatusSuccess, "alpha", "1.1.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, gradleProfileID, types.JobTypePreGate, types.JobStatusSuccess, "alpha", "2.0.0")

	has, err := db.HasSBOMEvidenceForStack(ctx, HasSBOMEvidenceForStackParams{
		Lang:    "java",
		Release: "17",
		Tool:    "maven",
	})
	if err != nil {
		t.Fatalf("HasSBOMEvidenceForStack(maven) failed: %v", err)
	}
	if has {
		t.Fatal("HasSBOMEvidenceForStack(maven)=true, want false before a successful allowed gate job exists for that stack")
	}

	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypeReGate, types.JobStatusSuccess, "alpha", "3.0.0")

	has, err = db.HasSBOMEvidenceForStack(ctx, HasSBOMEvidenceForStackParams{
		Lang:    "java",
		Release: "17",
		Tool:    "maven",
	})
	if err != nil {
		t.Fatalf("HasSBOMEvidenceForStack(maven, second read) failed: %v", err)
	}
	if !has {
		t.Fatal("HasSBOMEvidenceForStack(maven)=false, want true after a successful allowed gate job exists for that stack")
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

func TestListSBOMCompatRows_FiltersByGateStatusTypeStackAndRequestedLibs(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/sbom-compat-list", "main", "feature", []byte(`{"type":"sbom-compat-list"}`))

	mavenStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "maven", "example.com/java:17-maven")
	gradleStackID := upsertStackForSBOMCompatTest(t, ctx, db, "java", "17", "gradle", "example.com/java:17-gradle")

	mavenProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, mavenStackID, "3333333333333333333333333333333333333333")
	gradleProfileID := upsertGateProfileForSBOMCompatTest(t, ctx, db, fx.RunRepo.RepoID, gradleStackID, "4444444444444444444444444444444444444444")

	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePreGate, types.JobStatusSuccess, "alpha", "1.2.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePostGate, types.JobStatusSuccess, "alpha", "1.3.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypeReGate, types.JobStatusSuccess, "alpha", "1.2.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePreGate, types.JobStatusSuccess, "beta", "2.0.0")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePreGate, types.JobStatusSuccess, "org:lib", "3.1.0")

	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypeMod, types.JobStatusSuccess, "alpha", "9.9.9")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, mavenProfileID, types.JobTypePreGate, types.JobStatusFail, "beta", "9.9.9")
	createGateJobAndSBOMRowForSBOMCompatTest(t, ctx, db, fx, gradleProfileID, types.JobTypePreGate, types.JobStatusSuccess, "alpha", "8.0.0")

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
		{Lib: "alpha", Ver: "1.2.0"},
		{Lib: "alpha", Ver: "1.3.0"},
		{Lib: "beta", Ver: "2.0.0"},
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
	jobType types.JobType,
	status types.JobStatus,
	lib, ver string,
) {
	t.Helper()

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       fx.Run.ID,
		RepoID:      fx.RunRepo.RepoID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     1,
		Name:        "sbom-" + lib + "-" + ver + "-" + string(jobType) + "-" + string(status),
		Status:      status,
		JobType:     jobType,
		JobImage:    "example.com/gate:latest",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s %s %s): %v", lib, ver, jobType, err)
	}

	if err := db.UpsertGateJobProfileLink(ctx, UpsertGateJobProfileLinkParams{
		JobID:     job.ID.String(),
		ProfileID: profileID,
	}); err != nil {
		t.Fatalf("UpsertGateJobProfileLink(job=%s): %v", job.ID, err)
	}

	if err := db.UpsertSBOMRow(ctx, UpsertSBOMRowParams{
		JobID:  job.ID,
		RepoID: fx.RunRepo.RepoID,
		Lib:    lib,
		Ver:    ver,
	}); err != nil {
		t.Fatalf("UpsertSBOMRow(job=%s, lib=%s, ver=%s): %v", job.ID, lib, ver, err)
	}
}
