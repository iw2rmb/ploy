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

func TestListRunsHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	modID := domaintypes.NewModID()
	specID := domaintypes.NewSpecID()
	st := &mockStore{
		listRunsResult: []store.Run{
			{
				ID:        runID,
				ModID:     modID,
				SpecID:    specID,
				Status:    store.RunStatusStarted,
				CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	rr := httptest.NewRecorder()

	listRunsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Runs []RunSummary `json:"runs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}
	if resp.Runs[0].ID.String() != runIDStr {
		t.Fatalf("unexpected run id: %s", resp.Runs[0].ID.String())
	}
}

func TestGetRunHandler_Success_WithCounts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	modID := domaintypes.NewModID()
	specID := domaintypes.NewSpecID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     modID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusQueued, Count: 1},
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runIDStr, nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	getRunHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID.String() != runIDStr {
		t.Fatalf("unexpected run id: %s", resp.ID.String())
	}
	if resp.Counts == nil || resp.Counts.Total != 2 {
		t.Fatalf("expected counts total=2, got %+v", resp.Counts)
	}
}

// TestCancelRunHandlerV1_CancelsRunAndWork verifies that POST /v1/runs/{id}/cancel
// performs transactional cancellation via store.CancelRunV1.
func TestCancelRunHandlerV1_CancelsRunAndWork(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	modID := domaintypes.NewModID()
	specID := domaintypes.NewSpecID()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     modID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/cancel", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.cancelRunV1Called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
	if st.cancelRunV1Param != runIDStr {
		t.Fatalf("expected CancelRunV1 run id %q, got %q", runIDStr, st.cancelRunV1Param)
	}
	if st.updateRunStatusCalled {
		t.Fatalf("did not expect UpdateRunStatus to be called directly")
	}
	if st.updateRunRepoStatusCalled {
		t.Fatalf("did not expect UpdateRunRepoStatus to be called directly")
	}
	if st.updateJobStatusCalled {
		t.Fatalf("did not expect UpdateJobStatus to be called directly")
	}
}

func TestCancelRunHandlerV1_CancelRunV1Error(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		cancelRunV1Err: errors.New("db exploded"),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/cancel", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.cancelRunV1Called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
}

func TestCancelRunHandlerV1_TerminalRunIsIdempotent(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusCancelled,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/cancel", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.cancelRunV1Called {
		t.Fatalf("did not expect CancelRunV1 to be called for terminal run")
	}
}

func TestAddRunRepoHandler_CreatesRepoAndJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     "mod_1",
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		createModRepoResult: store.ModRepo{
			ID:        repoID,
			ModID:     "mod_1",
			RepoUrl:   "https://github.com/org/repo.git",
			BaseRef:   "main",
			TargetRef: "feature",
		},
	}

	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/repos", bytes.NewReader(body))
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	addRunRepoHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.createModRepoCalled || !st.createRunRepoCalled {
		t.Fatalf("expected CreateModRepo and CreateRunRepo to be called")
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs to be created for new repo, got %d", st.createJobCallCount)
	}
}

func TestListRunReposHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	repoID := domaintypes.NewModRepoID()

	st := &mockStore{
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				Status:        store.RunRepoStatusQueued,
				Attempt:       1,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				RepoUrl:       "https://github.com/org/repo.git",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runIDStr+"/repos", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RunRepoResponse `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Repos) != 1 || resp.Repos[0].RepoID != repoID || resp.Repos[0].RepoURL != "https://github.com/org/repo.git" {
		t.Fatalf("unexpected repos response: %+v", resp.Repos)
	}
	if !st.listRunReposWithURLByRunCalled {
		t.Fatalf("expected ListRunReposWithURLByRun to be called")
	}
	if st.listRunReposWithURLByRunParam != runIDStr {
		t.Fatalf("expected run id %q, got %q", runIDStr, st.listRunReposWithURLByRunParam)
	}
	if st.listRunReposByRunCalled {
		t.Fatalf("did not expect ListRunReposByRun to be called")
	}
	if st.getModRepoCalled {
		t.Fatalf("did not expect GetModRepo to be called")
	}
}

func TestListRunReposHandler_ListError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()

	st := &mockStore{
		listRunReposWithURLByRunErr: errors.New("db exploded"),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runIDStr+"/repos", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelRunRepoHandlerV1_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	st := &mockStore{
		getRunRepoErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	cancelRunRepoHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}
