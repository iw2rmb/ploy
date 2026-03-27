package handlers

import (
	"context"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestMaybePersistGateSuccessSBOMRows_PersistsRowsForSuccessfulGate(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	repoID := domaintypes.NewRepoID()
	objKey := "artifacts/run/" + runID.String() + "/bundle/sbom.tar.gz"
	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoID,
		JobType: domaintypes.JobTypePreGate,
	}

	st := &mockStore{
		listArtifactBundlesMetaByRunAndJobResult: []store.ArtifactBundle{
			{RunID: runID, JobID: &jobID, ObjectKey: &objKey},
		},
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

	count, err := maybePersistGateSuccessSBOMRows(context.Background(), st, bp, job, domaintypes.JobStatusSuccess)
	if err != nil {
		t.Fatalf("maybePersistGateSuccessSBOMRows error: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted row count = %d, want 1", count)
	}
	assertCalled(t, "DeleteSBOMRowsByJob", st.deleteSBOMRowsByJobCalled)
	if st.deleteSBOMRowsByJobParam != jobID {
		t.Fatalf("deleteSBOMRowsByJob job_id = %q, want %q", st.deleteSBOMRowsByJobParam, jobID)
	}
	if len(st.upsertSBOMRowParams) != 1 {
		t.Fatalf("upsertSBOMRow params count = %d, want 1", len(st.upsertSBOMRowParams))
	}
	got := st.upsertSBOMRowParams[0]
	if got.JobID != jobID || got.RepoID != repoID || got.Lib != "org.example:lib-a" || got.Ver != "1.0.0" {
		t.Fatalf("unexpected upsert row: %+v", got)
	}
}

func TestMaybePersistGateSuccessSBOMRows_SkipsNonGateOrNonSuccess(t *testing.T) {
	t.Parallel()

	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   domaintypes.NewRunID(),
		RepoID:  domaintypes.NewRepoID(),
		JobType: domaintypes.JobTypeMod,
	}
	st := &mockStore{}

	count, err := maybePersistGateSuccessSBOMRows(context.Background(), st, nil, job, domaintypes.JobStatusSuccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	count, err = maybePersistGateSuccessSBOMRows(context.Background(), st, nil, job, domaintypes.JobStatusFail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if st.deleteSBOMRowsByJobCalled || st.upsertSBOMRowCalled {
		t.Fatal("expected no sbom persistence calls")
	}
}
