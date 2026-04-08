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

func TestSBOMJobIsMoreRecent_PrefersMostRecentFinishedAtOverLexicographicID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	olderLexicographicallyGreaterID := domaintypes.JobID("job-z-older")
	newerLexicographicallySmallerID := domaintypes.JobID("job-a-newer")

	candidate := store.Job{
		ID:      newerLexicographicallySmallerID,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
		FinishedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
	}
	current := store.Job{
		ID:      olderLexicographicallyGreaterID,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
		FinishedAt: pgtype.Timestamptz{
			Time:  now.Add(-1 * time.Minute),
			Valid: true,
		},
	}
	if !sbomJobIsMoreRecent(candidate, current) {
		t.Fatalf("expected candidate %s to be more recent than %s", candidate.ID, current.ID)
	}
}

func TestSBOMJobIsMoreRecent_UsesIDAsDeterministicTieBreak(t *testing.T) {
	t.Parallel()

	finishedAt := time.Now().UTC()
	candidate := store.Job{
		ID:      domaintypes.JobID("job-z"),
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
		FinishedAt: pgtype.Timestamptz{
			Time:  finishedAt,
			Valid: true,
		},
	}
	current := store.Job{
		ID:      domaintypes.JobID("job-a"),
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
		FinishedAt: pgtype.Timestamptz{
			Time:  finishedAt,
			Valid: true,
		},
	}

	if !sbomJobIsMoreRecent(candidate, current) {
		t.Fatalf("expected candidate %s to win deterministic tie break over %s", candidate.ID, current.ID)
	}
}
