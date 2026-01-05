package handlers

import (
	"bytes"
	"encoding/json"
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

	runID := domaintypes.NewRunID().String()
	st := &mockStore{
		listRunsResult: []store.Run{
			{
				ID:        runID,
				ModID:     "mod_1",
				SpecID:    "spec_1",
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
	if resp.Runs[0].ID.String() != runID {
		t.Fatalf("unexpected run id: %s", resp.Runs[0].ID.String())
	}
}

func TestGetRunHandler_Success_WithCounts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     "mod_1",
			SpecID:    "spec_1",
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusQueued, Count: 1},
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	getRunHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID.String() != runID {
		t.Fatalf("unexpected run id: %s", resp.ID.String())
	}
	if resp.Counts == nil || resp.Counts.Total != 2 {
		t.Fatalf("expected counts total=2, got %+v", resp.Counts)
	}
}

// TestCancelRunHandlerV1_CancelsRunAndWork verifies that POST /v1/runs/{id}/cancel
// cancels the run, cancels Queued/Running repos, and cancels Created/Queued/Running jobs.
// Required by roadmap/v1/scope.md:72 and roadmap/v1/statuses.md:177-184.
func TestCancelRunHandlerV1_CancelsRunAndWork(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     "mod_1",
			SpecID:    "spec_1",
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		listRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusQueued},
			{RunID: runID, RepoID: "repo_done", Status: store.RunRepoStatusSuccess},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), RunID: runID, Status: store.JobStatusCreated},
			{ID: domaintypes.NewJobID().String(), RunID: runID, Status: store.JobStatusSuccess},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/cancel", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunStatusCalled {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.ID != runID || st.updateRunStatusParams.Status != store.RunStatusCancelled {
		t.Fatalf("unexpected UpdateRunStatus params: %+v", st.updateRunStatusParams)
	}
	// Should cancel the Queued repo (not the Success repo).
	if len(st.updateRunRepoStatusParams) != 1 {
		t.Fatalf("expected 1 repo status update (for Queued repo), got %d", len(st.updateRunRepoStatusParams))
	}
	// Should cancel the Created job (not the Success job).
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected 1 job status update (for Created job), got %d", len(st.updateJobStatusCalls))
	}
}

func TestAddRunRepoHandler_CreatesRepoAndJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()
	specID := domaintypes.NewSpecID().String()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     "mod_1",
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{}`)},
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
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/repos", bytes.NewReader(body))
	req.SetPathValue("id", runID)
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

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()

	st := &mockStore{
		listRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: repoID, RepoBaseRef: "main", RepoTargetRef: "feature", Status: store.RunRepoStatusQueued, Attempt: 1},
		},
		getModRepoResult: store.ModRepo{ID: repoID, RepoUrl: "https://github.com/org/repo.git"},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/repos", nil)
	req.SetPathValue("id", runID)
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
}

func TestDeleteRunRepoHandler_NotFound(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getRunRepoErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/run_1/repos/repo_1", nil)
	req.SetPathValue("id", "run_1")
	req.SetPathValue("repo_id", "repo_1")
	rr := httptest.NewRecorder()

	deleteRunRepoHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}
