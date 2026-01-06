package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/store"
)

// -------------------------------------------------------------------------
// Tests for POST /v1/runs/{run_id}/pull
// -------------------------------------------------------------------------

func TestPullRunRepoHandler_Success(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getRunResult: store.Run{ID: "run_123", ModID: "mod_1"},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         "run_123",
				RepoID:        "repo_abc",
				RepoTargetRef: "feature-branch",
				RepoUrl:       "https://github.com/org/repo.git",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with repo_url that matches (with .git suffix that normalizes away)
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != "run_123" {
		t.Fatalf("expected run_id 'run_123', got %q", resp.RunID)
	}
	if resp.RepoID != "repo_abc" {
		t.Fatalf("expected repo_id 'repo_abc', got %q", resp.RepoID)
	}
	if resp.RepoTargetRef != "feature-branch" {
		t.Fatalf("expected repo_target_ref 'feature-branch', got %q", resp.RepoTargetRef)
	}

	if !st.getRunCalled {
		t.Fatalf("expected GetRun to be called")
	}
	if !st.listRunReposWithURLByRunCalled {
		t.Fatalf("expected ListRunReposWithURLByRun to be called")
	}
	if st.listRunReposWithURLByRunParam != "run_123" {
		t.Fatalf("expected run_id 'run_123', got %q", st.listRunReposWithURLByRunParam)
	}
}

func TestPullRunRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	// Test that .git suffix normalization works.
	// Server stores URL without .git, client sends with .git.
	st := &mockStore{
		getRunResult: store.Run{ID: "run_123"},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         "run_123",
				RepoID:        "repo_abc",
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo", // stored without .git
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends with .git
	body := `{"repo_url": "https://github.com/org/repo.git"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_URLNormalization_TrailingSlash(t *testing.T) {
	t.Parallel()

	// Test that trailing slash normalization works.
	st := &mockStore{
		getRunResult: store.Run{ID: "run_123"},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         "run_123",
				RepoID:        "repo_abc",
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo/",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends without trailing slash
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_RunNotFound(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/nonexistent/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "nonexistent")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_RepoNotFound(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getRunResult: store.Run{ID: "run_123"},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         "run_123",
				RepoID:        "repo_abc",
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/other-repo",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with non-matching repo_url
	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_MultipleMatches(t *testing.T) {
	t.Parallel()

	// This shouldn't happen in practice (mod_repos has unique constraint on
	// (mod_id, repo_url)), but the handler should return an error if it does.
	st := &mockStore{
		getRunResult: store.Run{ID: "run_123"},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         "run_123",
				RepoID:        "repo_1",
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo",
			},
			{
				RunID:         "run_123",
				RepoID:        "repo_2",
				RepoTargetRef: "develop",
				RepoUrl:       "https://github.com/org/repo.git", // same after normalization
			},
		},
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := pullRunRepoHandler(st)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_MissingRunID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs//pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_StoreError(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getRunResult:                store.Run{ID: "run_123"},
		listRunReposWithURLByRunErr: errors.New("database error"),
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", "run_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// -------------------------------------------------------------------------
// Tests for POST /v1/mods/{mod_id}/pull
// -------------------------------------------------------------------------

func TestPullModRepoHandler_Success_LastSucceeded(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123", Name: "test-mod"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        "repo_abc",
				ModID:     "mod_123",
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         "run_456",
			RepoID:        "repo_abc",
			RepoTargetRef: "feature-branch",
		},
	}
	handler := pullModRepoHandler(st)

	// Default mode is "last-succeeded"
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != "run_456" {
		t.Fatalf("expected run_id 'run_456', got %q", resp.RunID)
	}
	if resp.RepoID != "repo_abc" {
		t.Fatalf("expected repo_id 'repo_abc', got %q", resp.RepoID)
	}
	if resp.RepoTargetRef != "feature-branch" {
		t.Fatalf("expected repo_target_ref 'feature-branch', got %q", resp.RepoTargetRef)
	}

	// Verify the store call used the correct status filter
	if !st.getLatestRunRepoByModAndRepoStatusCalled {
		t.Fatalf("expected GetLatestRunRepoByModAndRepoStatus to be called")
	}
	if st.getLatestRunRepoByModAndRepoStatusParams.Status != store.RunRepoStatusSuccess {
		t.Fatalf("expected status filter 'Success', got %q", st.getLatestRunRepoByModAndRepoStatusParams.Status)
	}
}

func TestPullModRepoHandler_Success_LastFailed(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123", Name: "test-mod"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        "repo_abc",
				ModID:     "mod_123",
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         "run_789",
			RepoID:        "repo_abc",
			RepoTargetRef: "bugfix-branch",
		},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "last-failed"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != "run_789" {
		t.Fatalf("expected run_id 'run_789', got %q", resp.RunID)
	}

	// Verify the store call used the correct status filter
	if st.getLatestRunRepoByModAndRepoStatusParams.Status != store.RunRepoStatusFail {
		t.Fatalf("expected status filter 'Fail', got %q", st.getLatestRunRepoByModAndRepoStatusParams.Status)
	}
}

func TestPullModRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        "repo_abc",
				ModID:     "mod_123",
				RepoUrl:   "https://github.com/org/repo.git", // with .git
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         "run_456",
			RepoID:        "repo_abc",
			RepoTargetRef: "feature",
		},
	}
	handler := pullModRepoHandler(st)

	// Request without .git suffix
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_ModNotFound(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/nonexistent/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "nonexistent")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_RepoNotInMod(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        "repo_abc",
				ModID:     "mod_123",
				RepoUrl:   "https://github.com/org/other-repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_NoMatchingRun(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        "repo_abc",
				ModID:     "mod_123",
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusErr: pgx.ErrNoRows,
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_InvalidMode(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult: store.Mod{ID: "mod_123"},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := pullModRepoHandler(st)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_MissingModID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods//pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_StoreError(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		getModResult:         store.Mod{ID: "mod_123"},
		listModReposByModErr: errors.New("database error"),
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod_123/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", "mod_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}
