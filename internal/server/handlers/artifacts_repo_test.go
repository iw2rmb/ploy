package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunRepoArtifactsHandler_Success_FiltersAndOrders(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()

	job1 := domaintypes.NewJobID().String()
	job2 := domaintypes.NewJobID().String()
	otherJob := domaintypes.NewJobID().String()
	runIDTyped := domaintypes.RunID(runID)
	repoIDTyped := domaintypes.ModRepoID(repoID)
	job1Typed := domaintypes.JobID(job1)
	job2Typed := domaintypes.JobID(job2)
	otherJobTyped := domaintypes.JobID(otherJob)

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
			RunID:   runIDTyped,
			RepoID:  repoIDTyped,
			Status:  store.RunRepoStatusRunning,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{ID: job1Typed, RunID: runIDTyped, RepoID: repoIDTyped, Attempt: 1, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
			{ID: job2Typed, RunID: runIDTyped, RepoID: repoIDTyped, Attempt: 1, Meta: withNextIDMeta([]byte(`{}`), float64(2000))},
		},
		listArtifactBundlesMetaByRunResult: []store.ArtifactBundle{
			{
				ID:         pgtype.UUID{Bytes: id2, Valid: true},
				RunID:      runIDTyped,
				JobID:      &job1Typed,
				Name:       &name2,
				Cid:        &cid2,
				Digest:     &digest2,
				CreatedAt:  pgtype.Timestamptz{Time: t2, Valid: true},
				BundleSize: 2,
			},
			{
				ID:         pgtype.UUID{Bytes: id1, Valid: true},
				RunID:      runIDTyped,
				JobID:      &job1Typed,
				Name:       &name1,
				Cid:        &cid1,
				Digest:     &digest1,
				CreatedAt:  pgtype.Timestamptz{Time: t1, Valid: true},
				BundleSize: 1,
			},
			{
				ID:         pgtype.UUID{Bytes: idOther, Valid: true},
				RunID:      runIDTyped,
				JobID:      &otherJobTyped,
				Name:       nil,
				Cid:        &cidOther,
				Digest:     &digestOther,
				CreatedAt:  pgtype.Timestamptz{Time: t1, Valid: true},
				BundleSize: 3,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/repos/"+repoID+"/artifacts", nil)
	req.SetPathValue("run_id", runID)
	req.SetPathValue("repo_id", repoID)
	rr := httptest.NewRecorder()

	listRunRepoArtifactsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.getRunRepoCalled {
		t.Fatalf("expected GetRunRepo to be called")
	}
	if !st.listJobsByRunRepoAttemptCalled {
		t.Fatalf("expected ListJobsByRunRepoAttempt to be called")
	}
	if !st.listArtifactBundlesMetaByRunCalled {
		t.Fatalf("expected ListArtifactBundlesMetaByRun to be called")
	}

	var resp struct {
		Artifacts []struct {
			ID string `json:"id"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(resp.Artifacts))
	}
	// Expected ordering:
	// - job1 next_id=1000, created_at ascending (t1 then t2).
	if resp.Artifacts[0].ID != id1.String() || resp.Artifacts[1].ID != id2.String() {
		t.Fatalf("unexpected artifact order: %+v", resp.Artifacts)
	}
}
