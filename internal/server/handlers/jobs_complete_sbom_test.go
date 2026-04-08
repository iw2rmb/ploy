package handlers

import (
	"context"
	"testing"
	"time"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMaybePersistLatestSuccessfulCycleSBOMRows_PersistsRowsFromLatestSuccessfulSBOMJob(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	attempt := int32(1)
	now := time.Now().UTC()
	sbomOldSuccessID := domaintypes.JobID("job-pre-sbom")
	sbomFailID := domaintypes.JobID("job-post-sbom")
	sbomLatestSuccessID := domaintypes.JobID("job-regate-sbom")
	migID := domaintypes.JobID("job-mig")

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID: sbomOldSuccessID, RunID: runID, RepoID: repoID, Attempt: attempt,
			JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true},
		},
		{ID: migID, RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess},
		{
			ID: sbomFailID, RunID: runID, RepoID: repoID, Attempt: attempt,
			JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusFail,
			FinishedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Minute), Valid: true},
		},
		{
			ID: sbomLatestSuccessID, RunID: runID, RepoID: repoID, Attempt: attempt,
			JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	objKey := "artifacts/run/" + runID.String() + "/bundle/sbom.tar.gz"
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{RunID: runID, JobID: &sbomLatestSuccessID, ObjectKey: &objKey},
	}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/sbom.spdx.json": []byte(`{
  "spdxVersion":"SPDX-2.3",
  "packages":[{"name":"org.example:lib-a","versionInfo":"1.0.0"}]
}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(st, bs)

	count, err := maybePersistLatestSuccessfulCycleSBOMRows(context.Background(), st, bp, runID, repoID, attempt)
	if err != nil {
		t.Fatalf("maybePersistLatestSuccessfulCycleSBOMRows error: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted row count = %d, want 1", count)
	}

	assertCalled(t, "DeleteSBOMRowsByJob", st.deleteSBOMRowsByJob.called)
	if len(st.deleteSBOMRowsByJob.calls) != 3 {
		t.Fatalf("DeleteSBOMRowsByJob calls = %d, want 3 (sbom jobs)", len(st.deleteSBOMRowsByJob.calls))
	}

	if len(st.upsertSBOMRow.calls) != 1 {
		t.Fatalf("upsertSBOMRow params count = %d, want 1", len(st.upsertSBOMRow.calls))
	}
	got := st.upsertSBOMRow.calls[0]
	if got.JobID != sbomLatestSuccessID || got.RepoID != repoID || got.Lib != "org.example:lib-a" || got.Ver != "1.0.0" {
		t.Fatalf("unexpected upsert row: %+v", got)
	}
}

func TestMaybePersistLatestSuccessfulCycleSBOMRows_SkipsWhenNoSuccessfulSBOMJob(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	attempt := int32(1)

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: domaintypes.JobID("job-pre-sbom"), RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusFail},
		{ID: domaintypes.JobID("job-post-sbom"), RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusCancelled},
		{ID: domaintypes.JobID("job-mig"), RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess},
	}

	count, err := maybePersistLatestSuccessfulCycleSBOMRows(context.Background(), st, nil, runID, repoID, attempt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if st.deleteSBOMRowsByJob.called || st.upsertSBOMRow.called {
		t.Fatal("expected no sbom persistence calls")
	}
}

func TestMaybePersistSBOMRowsForJob_PersistsRowsForCompletedSBOMJob(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.JobID("job-pre-sbom")

	st := &jobStore{}
	objKey := "artifacts/run/" + runID.String() + "/bundle/sbom.tar.gz"
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/sbom.spdx.json": []byte(`{
  "spdxVersion":"SPDX-2.3",
  "packages":[{"name":"org.example:lib-a","versionInfo":"1.0.0"}]
}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(st, bs)

	count, err := maybePersistSBOMRowsForJob(context.Background(), st, bp, runID, repoID, jobID)
	if err != nil {
		t.Fatalf("maybePersistSBOMRowsForJob error: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted row count = %d, want 1", count)
	}
	if len(st.deleteSBOMRowsByJob.calls) != 1 || st.deleteSBOMRowsByJob.calls[0] != jobID {
		t.Fatalf("DeleteSBOMRowsByJob calls = %+v, want [%s]", st.deleteSBOMRowsByJob.calls, jobID)
	}
	if len(st.upsertSBOMRow.calls) != 1 {
		t.Fatalf("upsertSBOMRow params count = %d, want 1", len(st.upsertSBOMRow.calls))
	}
	got := st.upsertSBOMRow.calls[0]
	if got.JobID != jobID || got.RepoID != repoID || got.Lib != "org.example:lib-a" || got.Ver != "1.0.0" {
		t.Fatalf("unexpected upsert row: %+v", got)
	}
}

func TestLatestSuccessfulSBOMJob_PrefersMostRecentFinishedAtOverLexicographicID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	olderLexicographicallyGreaterID := domaintypes.JobID("job-z-older")
	newerLexicographicallySmallerID := domaintypes.JobID("job-a-newer")

	got, ok := latestSuccessfulSBOMJob([]store.Job{
		{
			ID:      olderLexicographicallyGreaterID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{
				Time:  now.Add(-1 * time.Minute),
				Valid: true,
			},
		},
		{
			ID:      newerLexicographicallySmallerID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{
				Time:  now,
				Valid: true,
			},
		},
	})
	if !ok {
		t.Fatal("expected successful sbom job")
	}
	if got.ID != newerLexicographicallySmallerID {
		t.Fatalf("selected job id = %s, want %s", got.ID, newerLexicographicallySmallerID)
	}
}

func TestLatestSuccessfulSBOMJob_UsesIDAsDeterministicTieBreak(t *testing.T) {
	t.Parallel()

	finishedAt := time.Now().UTC()
	lowID := domaintypes.JobID("job-a")
	highID := domaintypes.JobID("job-z")

	got, ok := latestSuccessfulSBOMJob([]store.Job{
		{
			ID: lowID, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
		},
		{
			ID: highID, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess,
			FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
		},
	})
	if !ok {
		t.Fatal("expected successful sbom job")
	}
	if got.ID != highID {
		t.Fatalf("selected job id = %s, want %s", got.ID, highID)
	}
}
