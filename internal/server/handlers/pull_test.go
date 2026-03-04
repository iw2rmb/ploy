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
	modID := domaintypes.NewMigID()
	repoID := domaintypes.NewRepoID()

	st := &mockStore{
		getRunResult: store.Run{ID: runID, MigID: modID},
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
	repoID := domaintypes.NewRepoID()

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
	repoID := domaintypes.NewRepoID()

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
	repoID := domaintypes.NewRepoID()

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
	repoID1 := domaintypes.NewRepoID()
	repoID2 := domaintypes.NewRepoID()

	// This shouldn't happen in practice (mig_repos has unique constraint on
	// (mig_id, repo_url)), but the handler should return an error if it does.
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
// Tests for POST /v1/migs/{mig_id}/pull
// -------------------------------------------------------------------------

func TestPullModRepoHandler_Success_LastSucceeded(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()
	modRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID, Name: "test-mig"},
		listMigReposByModResult: []store.MigRepo{
			{
				ID:        modRepoID,
				MigID:     modID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByMigAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "feature-branch",
		},
	}
	handler := pullMigRepoHandler(st)

	// Default mode is "last-succeeded"
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
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
		t.Fatalf("expected GetLatestRunRepoByMigAndRepoStatus to be called")
	}
	if st.getLatestRunRepoByModAndRepoStatusParams.Status != domaintypes.RunRepoStatusSuccess {
		t.Fatalf("expected status filter 'Success', got %q", st.getLatestRunRepoByModAndRepoStatusParams.Status)
	}
}

func TestPullModRepoHandler_Success_LastFailed(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()
	modRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID, Name: "test-mig"},
		listMigReposByModResult: []store.MigRepo{
			{
				ID:        modRepoID,
				MigID:     modID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByMigAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "bugfix-branch",
		},
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "last-failed"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
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
	if st.getLatestRunRepoByModAndRepoStatusParams.Status != domaintypes.RunRepoStatusFail {
		t.Fatalf("expected status filter 'Fail', got %q", st.getLatestRunRepoByModAndRepoStatusParams.Status)
	}
}

func TestPullModRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()
	modRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID},
		listMigReposByModResult: []store.MigRepo{
			{
				ID:        modRepoID,
				MigID:     modID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo.git"},
		},
		getLatestRunRepoByModAndRepoStatusResult: store.GetLatestRunRepoByMigAndRepoStatusRow{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "feature",
		},
	}
	handler := pullMigRepoHandler(st)

	// Request without .git suffix
	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_ModNotFound(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_RepoNotInMod(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()
	modRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID},
		listMigReposByModResult: []store.MigRepo{
			{
				ID:        modRepoID,
				MigID:     modID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/other-repo"},
		},
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_NoMatchingRun(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()
	modRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID},
		listMigReposByModResult: []store.MigRepo{
			{
				ID:        modRepoID,
				MigID:     modID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
		getLatestRunRepoByModAndRepoStatusErr: pgx.ErrNoRows,
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_InvalidMode(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	st := &mockStore{
		getModResult: store.Mig{ID: modID},
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	st := &mockStore{}
	handler := pullMigRepoHandler(st)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_MissingModID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs//pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPullModRepoHandler_StoreError(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	st := &mockStore{
		getModResult:         store.Mig{ID: modID},
		listMigReposByModErr: errors.New("database error"),
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID.String()+"/pull", bytes.NewBufferString(body))
	req.SetPathValue("mig_id", modID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}
