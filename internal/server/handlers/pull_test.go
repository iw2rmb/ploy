package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// -------------------------------------------------------------------------
// Tests for POST /v1/runs/{run_id}/pull
// -------------------------------------------------------------------------

func TestPullRunRepoHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()

	st := &mockStore{
		getRunResult: store.Run{ID: runID, ModID: modID},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoTargetRef: "feature-branch",
				RepoUrl:       "https://github.com/org/repo.git",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with repo_url that matches (with .git suffix that normalizes away)
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID.String(), resp.RunID.String())
	}
	if resp.RepoID != repoID {
		t.Fatalf("expected repo_id %q, got %q", repoID.String(), resp.RepoID.String())
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
	if st.listRunReposWithURLByRunParam != runID.String() {
		t.Fatalf("expected run_id %q, got %q", runID.String(), st.listRunReposWithURLByRunParam)
	}
}

func TestPullRunRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()

	// Test that .git suffix normalization works.
	// Server stores URL without .git, client sends with .git.
	st := &mockStore{
		getRunResult: store.Run{ID: runID},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo", // stored without .git
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends with .git
	body := `{"repo_url": "https://github.com/org/repo.git"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_URLNormalization_TrailingSlash(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()

	// Test that trailing slash normalization works.
	st := &mockStore{
		getRunResult: store.Run{ID: runID},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo/",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends without trailing slash
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_RunNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_RepoNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()

	st := &mockStore{
		getRunResult: store.Run{ID: runID},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/other-repo",
			},
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with non-matching repo_url
	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_MultipleMatches(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID1 := domaintypes.NewModRepoID()
	repoID2 := domaintypes.NewModRepoID()

	// This shouldn't happen in practice (mod_repos has unique constraint on
	// (mod_id, repo_url)), but the handler should return an error if it does.
	st := &mockStore{
		getRunResult: store.Run{ID: runID},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID1,
				RepoTargetRef: "main",
				RepoUrl:       "https://github.com/org/repo",
			},
			{
				RunID:         runID,
				RepoID:        repoID2,
				RepoTargetRef: "develop",
				RepoUrl:       "https://github.com/org/repo.git", // same after normalization
			},
		},
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullRunRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &mockStore{}
	handler := pullRunRepoHandler(st)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
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

	runID := domaintypes.NewRunID()

	st := &mockStore{
		getRunResult:                store.Run{ID: runID},
		listRunReposWithURLByRunErr: errors.New("database error"),
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("run_id", runID.String())
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

	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID, Name: "test-mod"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        repoID,
				ModID:     modID,
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "feature-branch",
		},
	}
	handler := pullModRepoHandler(st)

	// Default mode is "last-succeeded"
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID.String(), resp.RunID.String())
	}
	if resp.RepoID != repoID {
		t.Fatalf("expected repo_id %q, got %q", repoID.String(), resp.RepoID.String())
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

	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID, Name: "test-mod"},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        repoID,
				ModID:     modID,
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "bugfix-branch",
		},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "last-failed"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID.String(), resp.RunID.String())
	}

	// Verify the store call used the correct status filter
	if st.getLatestRunRepoByModAndRepoStatusParams.Status != store.RunRepoStatusFail {
		t.Fatalf("expected status filter 'Fail', got %q", st.getLatestRunRepoByModAndRepoStatusParams.Status)
	}
}

func TestPullModRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        repoID,
				ModID:     modID,
				RepoUrl:   "https://github.com/org/repo.git", // with .git
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByModAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "feature",
		},
	}
	handler := pullModRepoHandler(st)

	// Request without .git suffix
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_ModNotFound(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()

	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_RepoNotInMod(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        repoID,
				ModID:     modID,
				RepoUrl:   "https://github.com/org/other-repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_NoMatchingRun(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()
	repoID := domaintypes.NewModRepoID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID},
		listModReposByModResult: []store.ModRepo{
			{
				ID:        repoID,
				ModID:     modID,
				RepoUrl:   "https://github.com/org/repo",
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		getLatestRunRepoByModAndRepoStatusErr: pgx.ErrNoRows,
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_InvalidMode(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()

	st := &mockStore{
		getModResult: store.Mod{ID: modID},
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewModID()

	st := &mockStore{}
	handler := pullModRepoHandler(st)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
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

	modID := domaintypes.NewModID()

	st := &mockStore{
		getModResult:         store.Mod{ID: modID},
		listModReposByModErr: errors.New("database error"),
	}
	handler := pullModRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mod_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}
