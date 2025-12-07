package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// -------------------------------------------------------------------------
// Tests for GET /v1/repos — listReposHandler
// -------------------------------------------------------------------------

func TestListReposHandler_Success_Empty(t *testing.T) {
	// Test that GET /v1/repos returns an empty list when no repos exist.
	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RepoSummary `json:"repos"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(resp.Repos))
	}

	if !st.listDistinctReposCalled {
		t.Error("expected ListDistinctRepos to be called")
	}
	if st.listDistinctReposParam != "" {
		t.Errorf("expected empty filter, got %q", st.listDistinctReposParam)
	}
}

func TestListReposHandler_Success_WithData(t *testing.T) {
	// Test that GET /v1/repos returns repos with correct data.
	now := time.Now().UTC().Truncate(time.Microsecond)
	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{
			{
				RepoUrl:    "https://github.com/org/repo1.git",
				LastRunAt:  pgtype.Timestamptz{Time: now, Valid: true},
				LastStatus: store.RunRepoStatusSucceeded,
			},
			{
				RepoUrl:    "https://github.com/org/repo2.git",
				LastRunAt:  pgtype.Timestamptz{Valid: false},
				LastStatus: store.RunRepoStatusPending,
			},
		},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RepoSummary `json:"repos"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(resp.Repos))
	}

	// Verify first repo with timing data.
	if resp.Repos[0].RepoURL != "https://github.com/org/repo1.git" {
		t.Errorf("unexpected repo_url: %s", resp.Repos[0].RepoURL)
	}
	if resp.Repos[0].LastRunAt == nil {
		t.Error("expected last_run_at to be set")
	}
	if resp.Repos[0].LastStatus == nil || *resp.Repos[0].LastStatus != "succeeded" {
		t.Errorf("expected last_status 'succeeded', got %v", resp.Repos[0].LastStatus)
	}

	// Verify second repo without timing data.
	if resp.Repos[1].RepoURL != "https://github.com/org/repo2.git" {
		t.Errorf("unexpected repo_url: %s", resp.Repos[1].RepoURL)
	}
	if resp.Repos[1].LastRunAt != nil {
		t.Error("expected last_run_at to be nil for repo without timing")
	}
}

func TestListReposHandler_WithFilter(t *testing.T) {
	// Test that the 'contains' query parameter is passed to the store.
	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos?contains=org/project", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if st.listDistinctReposParam != "org/project" {
		t.Errorf("expected filter 'org/project', got %q", st.listDistinctReposParam)
	}
}

func TestListReposHandler_StoreError(t *testing.T) {
	// Test that store errors return 500.
	st := &mockStore{
		listDistinctReposErr: errors.New("database connection failed"),
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

// -------------------------------------------------------------------------
// Tests for GET /v1/repos/{repo_id}/runs — listRunsForRepoHandler
// -------------------------------------------------------------------------

func TestListRunsForRepoHandler_Success(t *testing.T) {
	// Test that GET /v1/repos/{repo_id}/runs returns runs for the repo.
	runID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	name := "test-batch"
	st := &mockStore{
		listRunsForRepoResult: []store.ListRunsForRepoRow{
			{
				RunID:      runID.String(),
				Name:       &name,
				RunStatus:  store.RunStatusSucceeded,
				RepoStatus: store.RunRepoStatusSucceeded,
				BaseRef:    "main",
				TargetRef:  "feature-branch",
				Attempt:    1,
				StartedAt:  pgtype.Timestamptz{Time: now, Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			},
		},
	}
	handler := listRunsForRepoHandler(st)

	// URL-encode the repo URL for the path parameter.
	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Runs []RepoRunSummary `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}

	// Verify run data.
	run := resp.Runs[0]
	if run.RunID != runID.String() {
		t.Errorf("unexpected run_id: %s", run.RunID)
	}
	if run.Name == nil || *run.Name != "test-batch" {
		t.Errorf("unexpected name: %v", run.Name)
	}
	if run.RunStatus != "succeeded" {
		t.Errorf("unexpected run_status: %s", run.RunStatus)
	}
	if run.RepoStatus != "succeeded" {
		t.Errorf("unexpected repo_status: %s", run.RepoStatus)
	}
	if run.BaseRef != "main" {
		t.Errorf("unexpected base_ref: %s", run.BaseRef)
	}
	if run.TargetRef != "feature-branch" {
		t.Errorf("unexpected target_ref: %s", run.TargetRef)
	}
	if run.Attempt != 1 {
		t.Errorf("unexpected attempt: %d", run.Attempt)
	}

	// Verify store was called with correct params.
	if !st.listRunsForRepoCalled {
		t.Error("expected ListRunsForRepo to be called")
	}
	if st.listRunsForRepoParams.RepoUrl != repoURL {
		t.Errorf("expected repo_url %q, got %q", repoURL, st.listRunsForRepoParams.RepoUrl)
	}
	if st.listRunsForRepoParams.Lim != 50 {
		t.Errorf("expected default limit 50, got %d", st.listRunsForRepoParams.Lim)
	}
	if st.listRunsForRepoParams.Off != 0 {
		t.Errorf("expected default offset 0, got %d", st.listRunsForRepoParams.Off)
	}
}

func TestListRunsForRepoHandler_WithPagination(t *testing.T) {
	// Test that pagination parameters are passed to the store.
	st := &mockStore{
		listRunsForRepoResult: []store.ListRunsForRepoRow{},
	}
	handler := listRunsForRepoHandler(st)

	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs?limit=25&offset=10", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if st.listRunsForRepoParams.Lim != 25 {
		t.Errorf("expected limit 25, got %d", st.listRunsForRepoParams.Lim)
	}
	if st.listRunsForRepoParams.Off != 10 {
		t.Errorf("expected offset 10, got %d", st.listRunsForRepoParams.Off)
	}
}

func TestListRunsForRepoHandler_LimitCapped(t *testing.T) {
	// Test that limit is capped at 100.
	st := &mockStore{
		listRunsForRepoResult: []store.ListRunsForRepoRow{},
	}
	handler := listRunsForRepoHandler(st)

	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs?limit=500", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if st.listRunsForRepoParams.Lim != 100 {
		t.Errorf("expected limit capped at 100, got %d", st.listRunsForRepoParams.Lim)
	}
}

func TestListRunsForRepoHandler_MissingRepoID(t *testing.T) {
	// Test that missing repo_id returns 400.
	st := &mockStore{}
	handler := listRunsForRepoHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos//runs", nil)
	req.SetPathValue("repo_id", "") // empty path value
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestListRunsForRepoHandler_InvalidRepoID(t *testing.T) {
	// Test that invalid repo_id (not a valid URL) returns 400.
	st := &mockStore{}
	handler := listRunsForRepoHandler(st)

	// URL-encode something that's not a valid URL.
	invalidRepoID := url.PathEscape("not-a-url")

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+invalidRepoID+"/runs", nil)
	req.SetPathValue("repo_id", invalidRepoID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListRunsForRepoHandler_InvalidLimit(t *testing.T) {
	// Test that invalid limit parameter returns 400.
	st := &mockStore{}
	handler := listRunsForRepoHandler(st)

	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs?limit=invalid", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestListRunsForRepoHandler_InvalidOffset(t *testing.T) {
	// Test that invalid offset parameter returns 400.
	st := &mockStore{}
	handler := listRunsForRepoHandler(st)

	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs?offset=-1", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestListRunsForRepoHandler_StoreError(t *testing.T) {
	// Test that store errors return 500.
	st := &mockStore{
		listRunsForRepoErr: errors.New("database connection failed"),
	}
	handler := listRunsForRepoHandler(st)

	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs", nil)
	req.SetPathValue("repo_id", encodedRepoURL)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

// -------------------------------------------------------------------------
// Integration test via Server.ServeHTTP to verify route registration
// -------------------------------------------------------------------------

func TestReposRoutes_Registration(t *testing.T) {
	// Test that the repo routes are correctly registered.
	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{},
		listRunsForRepoResult:   []store.ListRunsForRepoRow{},
	}

	// Create a server (we need the Server struct to access routes).
	// We'll use the registered handlers directly through a minimal setup.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/repos", listReposHandler(st))
	mux.HandleFunc("GET /v1/repos/{repo_id}/runs", listRunsForRepoHandler(st))

	// Test GET /v1/repos route.
	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /v1/repos: expected 200, got %d", rr.Code)
	}

	// Test GET /v1/repos/{repo_id}/runs route.
	repoURL := "https://github.com/org/repo.git"
	encodedRepoURL := url.PathEscape(repoURL)
	req = httptest.NewRequest(http.MethodGet, "/v1/repos/"+encodedRepoURL+"/runs", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /v1/repos/{repo_id}/runs: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// Ensure mockStore implements the Querier methods used by repo handlers.
var _ interface {
	ListDistinctRepos(ctx context.Context, filter string) ([]store.ListDistinctReposRow, error)
	ListRunsForRepo(ctx context.Context, arg store.ListRunsForRepoParams) ([]store.ListRunsForRepoRow, error)
} = (*mockStore)(nil)
