package handlers

import (
	"context"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestMaybePersistLatestSuccessfulCycleSBOMRows_PersistsRowsFromLatestSuccessfulSBOMJob(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	attempt := int32(1)
	sbomOldSuccessID := domaintypes.JobID("job-pre-sbom")
	sbomFailID := domaintypes.JobID("job-post-sbom")
	sbomLatestSuccessID := domaintypes.JobID("job-regate-sbom")
	migID := domaintypes.JobID("job-mig")

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: sbomOldSuccessID, RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess},
		{ID: migID, RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess},
		{ID: sbomFailID, RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusFail},
		{ID: sbomLatestSuccessID, RunID: runID, RepoID: repoID, Attempt: attempt, JobType: domaintypes.JobTypeSBOM, Status: domaintypes.JobStatusSuccess},
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
