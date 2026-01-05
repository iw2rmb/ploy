package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestResumeRun_Started_IsIdempotent(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	st := &mockStore{
		getRunResult: store.Run{ID: runID, Status: store.RunStatusStarted},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+runID+"/resume", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	resumeRunHandler(st, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus")
	}
	if st.incrementRunRepoAttemptCalled {
		t.Fatal("did not expect IncrementRunRepoAttempt")
	}
}

func TestResumeRun_Finished_RestartsFailedRepos(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()
	specID := domaintypes.NewSpecID().String()

	specBytes := []byte(`{"mods":[{"image":"img1:latest"}]}`)

	st := &mockStore{
		getRunResult: store.Run{ID: runID, Status: store.RunStatusFinished, SpecID: specID},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: specBytes,
		},
		listRunReposByRunResult: []store.RunRepo{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				Status:        store.RunRepoStatusFail,
				Attempt:       1,
				CreatedAt:     pgtype.Timestamptz{Valid: true},
			},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        store.RunRepoStatusQueued,
			Attempt:       2,
			CreatedAt:     pgtype.Timestamptz{Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+runID+"/resume", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	resumeRunHandler(st, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus")
	}
	if st.updateRunStatusParams.Status != store.RunStatusStarted {
		t.Fatalf("expected run status Started, got %s", st.updateRunStatusParams.Status)
	}

	if !st.incrementRunRepoAttemptCalled {
		t.Fatal("expected IncrementRunRepoAttempt")
	}
	if st.incrementRunRepoAttemptParam.RunID != runID || st.incrementRunRepoAttemptParam.RepoID != repoID {
		t.Fatalf("unexpected IncrementRunRepoAttempt params: %+v", st.incrementRunRepoAttemptParam)
	}

	if !st.createJobCalled {
		t.Fatal("expected CreateJob to be called to recreate jobs")
	}
	if !st.updateRunResumeCalled {
		t.Fatal("expected UpdateRunResume to be called")
	}
}

func TestResumeRun_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	st := &mockStore{getRunErr: pgx.ErrNoRows}

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+runID+"/resume", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	resumeRunHandler(st, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestResumeRun_Conflict_WeirdState(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	st := &mockStore{getRunResult: store.Run{ID: runID, Status: store.RunStatus("Weird")}}

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+runID+"/resume", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	resumeRunHandler(st, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
