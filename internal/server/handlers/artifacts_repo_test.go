package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunRepoArtifactsHandler_Success_FiltersAndOrders(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	job1 := domaintypes.NewJobID()
	job2 := domaintypes.NewJobID()
	otherJob := domaintypes.NewJobID()

	t1 := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)

	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	idOther := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	name1 := "bundle-1"
	name2 := "bundle-2"
	cid1 := "bafy-one"
	cid2 := "bafy-two"
	digest1 := "sha256:one"
	digest2 := "sha256:two"
	digestOther := "sha256:other"
	cidOther := "bafy-other"

	st := &mockStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Status:  domaintypes.RunRepoStatusRunning,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{ID: job1, RunID: runID, RepoID: repoID, Attempt: 1, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
			{ID: job2, RunID: runID, RepoID: repoID, Attempt: 1, Meta: withNextIDMeta([]byte(`{}`), float64(2000))},
		},
		listArtifactBundlesMetaByRunResult: []store.ArtifactBundle{
			{
				ID:         pgtype.UUID{Bytes: id2, Valid: true},
				RunID:      runID,
				JobID:      &job1,
				Name:       &name2,
				Cid:        &cid2,
				Digest:     &digest2,
				CreatedAt:  pgtype.Timestamptz{Time: t2, Valid: true},
				BundleSize: 2,
			},
			{
				ID:         pgtype.UUID{Bytes: id1, Valid: true},
				RunID:      runID,
				JobID:      &job1,
				Name:       &name1,
				Cid:        &cid1,
				Digest:     &digest1,
				CreatedAt:  pgtype.Timestamptz{Time: t1, Valid: true},
				BundleSize: 1,
			},
			{
				ID:         pgtype.UUID{Bytes: idOther, Valid: true},
				RunID:      runID,
				JobID:      &otherJob,
				Name:       nil,
				Cid:        &cidOther,
				Digest:     &digestOther,
				CreatedAt:  pgtype.Timestamptz{Time: t1, Valid: true},
				BundleSize: 3,
			},
		},
	}

	rr := doRequest(t, listRunRepoArtifactsHandler(st), http.MethodGet,
		"/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/artifacts", nil,
		"run_id", runID.String(), "repo_id", repoID.String())
	assertStatus(t, rr, http.StatusOK)

	type listResp struct {
		Artifacts []struct {
			ID string `json:"id"`
		} `json:"artifacts"`
	}
	resp := decodeBody[listResp](t, rr)
	if len(resp.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(resp.Artifacts))
	}
	if resp.Artifacts[0].ID != id1.String() || resp.Artifacts[1].ID != id2.String() {
		t.Fatalf("unexpected artifact order: %+v", resp.Artifacts)
	}
}
