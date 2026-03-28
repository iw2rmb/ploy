package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const testRunRepoSHASeed = "0123456789abcdef0123456789abcdef01234567"

func TestCancelRunHandlerV1_CancelsRunAndWork(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.cancelRunV1.called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
	if st.cancelRunV1.params != runID.String() {
		t.Fatalf("expected CancelRunV1 run id %q, got %q", runID, st.cancelRunV1.params)
	}
}

func TestCancelRunHandlerV1_CancelRunV1Error(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}
	st.cancelRunV1.err = errors.New("db exploded")

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
	if !st.cancelRunV1.called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
}

func TestCancelRunHandlerV1_TerminalRunIsIdempotent(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusCancelled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if st.cancelRunV1.called {
		t.Fatalf("did not expect CancelRunV1 to be called for terminal run")
	}
}

func TestAddRunRepoHandler_CreatesRepoWithoutImmediateJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	modRepoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	st := &mockStore{
		createMigRepoResult: store.MigRepo{
			ID:        modRepoID,
			RepoID:    repoID,
			BaseRef:   "main",
			TargetRef: "feature",
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo.git"},
		},
	}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)}

	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos", bytes.NewReader(body))
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	addRunRepoHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusCreated)
	if !st.createMigRepoCalled || !st.createRunRepoCalled {
		t.Fatalf("expected CreateMigRepo and CreateRunRepo to be called")
	}
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs to be created for new repo submission, got %d", st.createJobCallCount)
	}
}

func TestListRunReposHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &mockStore{}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
			CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			RepoUrl:       "https://github.com/org/repo.git",
		},
		}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Repos []RunRepoResponse `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Repos) != 1 || resp.Repos[0].RepoID != repoID || resp.Repos[0].RepoURL != "https://github.com/org/repo.git" {
		t.Fatalf("unexpected repos response: %+v", resp.Repos)
	}
	if !st.listRunReposWithURLByRun.called {
		t.Fatalf("expected ListRunReposWithURLByRun to be called")
	}
	if st.listRunReposWithURLByRun.params != runID.String() {
		t.Fatalf("expected run id %q, got %q", runID, st.listRunReposWithURLByRun.params)
	}
}

func TestListRunReposHandler_ListError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.listRunReposWithURLByRun.err = errors.New("db exploded")

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestCancelRunRepoHandlerV1_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	st := &mockStore{
		getRunRepoErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	cancelRunRepoHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestRestartRunRepoHandler_ReopensTerminalRunAndCreatesJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	modRepoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	st := &mockStore{
		getRunRepoResults: []store.RunRepo{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				RepoSha0:      testRunRepoSHASeed,
				Attempt:       1,
				Status:        domaintypes.RunRepoStatusFail,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "develop",
				RepoTargetRef: "feature-2",
				RepoSha0:      testRunRepoSHASeed,
				Attempt:       2,
				Status:        domaintypes.RunRepoStatusQueued,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: modRepoID, RepoID: repoID},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo.git"},
		},
	}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    specID,
		Status:    domaintypes.RunStatusFinished,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)}
	st.getModRepo.val = store.MigRepo{
		ID:     modRepoID,
		RepoID: repoID,
		}

	reqBody := map[string]any{
		"base_ref":   "develop",
		"target_ref": "feature-2",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/restart", bytes.NewReader(body))
	req.SetPathValue("id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	restartRunRepoHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.updateRunStatus.called {
		t.Fatalf("expected UpdateRunStatus to be called for terminal run")
	}
	if !st.updateRunRepoRefs.called || !st.updateMigRepoRefs.called {
		t.Fatalf("expected refs updates to be called")
	}
	if !st.incrementRunRepoAttempt.called {
		t.Fatalf("expected IncrementRunRepoAttempt to be called")
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs for restarted repo, got %d", st.createJobCallCount)
	}
}

func TestStartRunHandler_StartsQueuedRepos(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()

	queuedRepo := store.RunRepo{
		RunID:         runID,
		RepoID:        repoID,
		RepoBaseRef:   "main",
		RepoTargetRef: "feature",
		RepoSha0:      testRunRepoSHASeed,
		Attempt:       1,
		Status:        domaintypes.RunRepoStatusQueued,
		CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	st := &mockStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)}
	st.listRunReposByRun.val = []store.RunRepo{queuedRepo}
	st.listQueuedRunReposByRun.val = []store.RunRepo{queuedRepo}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/start", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	startRunHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if st.createJobCallCount != 3 {
		t.Fatalf("expected starter to create 3 jobs, got %d", st.createJobCallCount)
	}

	resp := decodeBody[StartRunResponse](t, rr)
	if resp.RunID != runID {
		t.Fatalf("expected run id %q, got %q", runID, resp.RunID)
	}
	if resp.Started != 1 || resp.Pending != 1 || resp.AlreadyDone != 0 {
		t.Fatalf("unexpected start response: %+v", resp)
	}
}

func TestStartRunHandler_TerminalRunConflict(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusCancelled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/start", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	startRunHandler(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusConflict)
}
