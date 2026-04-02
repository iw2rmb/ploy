package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
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
	migID := domaintypes.NewMigID()
	repoID := domaintypes.NewRepoID()

	st := &runStore{}
	st.getRun.val = store.Run{ID: runID, MigID: migID}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "feature-branch",
			RepoUrl:       "https://github.com/org/repo.git",
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with repo_url that matches (with .git suffix that normalizes away)
	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusOK)

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

	if !st.getRun.called {
		t.Fatalf("expected GetRun to be called")
	}
	if !st.listRunReposWithURLByRun.called {
		t.Fatalf("expected ListRunReposWithURLByRun to be called")
	}
	if st.listRunReposWithURLByRun.params != runID.String() {
		t.Fatalf("expected run_id %q, got %q", runID.String(), st.listRunReposWithURLByRun.params)
	}
}

func TestPullRunRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	// Test that .git suffix normalization works.
	// Server stores URL without .git, client sends with .git.
	st := &runStore{}
	st.getRun.val = store.Run{ID: runID}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "main",
			RepoUrl:       "https://github.com/org/repo", // stored without .git
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends with .git
	body := `{"repo_url": "https://github.com/org/repo.git"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusOK)
}

func TestPullRunRepoHandler_URLNormalization_TrailingSlash(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	// Test that trailing slash normalization works.
	st := &runStore{}
	st.getRun.val = store.Run{ID: runID}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "main",
			RepoUrl:       "https://github.com/org/repo/",
		},
	}
	handler := pullRunRepoHandler(st)

	// Client sends without trailing slash
	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusOK)
}

func TestPullRunRepoHandler_RunNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &runStore{}
	st.getRun.err = pgx.ErrNoRows
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPullRunRepoHandler_RepoNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &runStore{}
	st.getRun.val = store.Run{ID: runID}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        repoID,
			RepoTargetRef: "main",
			RepoUrl:       "https://github.com/org/other-repo",
		},
	}
	handler := pullRunRepoHandler(st)

	// Request with non-matching repo_url
	body := `{"repo_url": "https://github.com/org/nonexistent"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPullRunRepoHandler_MultipleMatches(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID1 := domaintypes.NewRepoID()
	repoID2 := domaintypes.NewRepoID()

	// This shouldn't happen in practice (mig_repos has unique constraint on
	// (mig_id, repo_url)), but the handler should return an error if it does.
	st := &runStore{}
	st.getRun.val = store.Run{ID: runID}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
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
	}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusConflict)
}

func TestPullRunRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &runStore{}
	handler := pullRunRepoHandler(st)

	body := `{}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestPullRunRepoHandler_MissingRunID(t *testing.T) {
	t.Parallel()

	st := &runStore{}
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs//pull", body, "run_id", "")

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestPullRunRepoHandler_StoreError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &runStore{}
	st.getRun.val = store.Run{ID: runID}
	st.listRunReposWithURLByRun.err = errors.New("database error")
	handler := pullRunRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs/"+runID.String()+"/pull", body, "run_id", runID.String())

	assertStatus(t, rr, http.StatusInternalServerError)
}

// -------------------------------------------------------------------------
// Tests for POST /v1/migs/{mig_id}/pull
// -------------------------------------------------------------------------

func TestPullMigRepoHandler_Success_LastSucceeded(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	migRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID, Name: "test-mig"},
		listMigReposByMigResult: []store.MigRepo{
			{
				ID:        migRepoID,
				MigID:     migID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
	}
	st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{
		RunID:         runID,
		RepoID:        repoID,
		RepoTargetRef: "feature-branch",
	}
	handler := pullMigRepoHandler(st)

	// Default mode is "last-succeeded"
	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusOK)

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
	if !st.getLatestRunRepoByMigAndRepoStatus.called {
		t.Fatalf("expected GetLatestRunRepoByMigAndRepoStatus to be called")
	}
	if st.getLatestRunRepoByMigAndRepoStatus.params.Status != domaintypes.RunRepoStatusSuccess {
		t.Fatalf("expected status filter 'Success', got %q", st.getLatestRunRepoByMigAndRepoStatus.params.Status)
	}
}

func TestPullMigRepoHandler_Success_LastFailed(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	migRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID, Name: "test-mig"},
		listMigReposByMigResult: []store.MigRepo{
			{
				ID:        migRepoID,
				MigID:     migID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
	}
	st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{
		RunID:         runID,
		RepoID:        repoID,
		RepoTargetRef: "bugfix-branch",
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "last-failed"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp pullResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID.String(), resp.RunID.String())
	}

	// Verify the store call used the correct status filter
	if st.getLatestRunRepoByMigAndRepoStatus.params.Status != domaintypes.RunRepoStatusFail {
		t.Fatalf("expected status filter 'Fail', got %q", st.getLatestRunRepoByMigAndRepoStatus.params.Status)
	}
}

func TestPullMigRepoHandler_URLNormalization(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	migRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID},
		listMigReposByMigResult: []store.MigRepo{
			{
				ID:        migRepoID,
				MigID:     migID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo.git"},
		},
	}
	st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{
		RunID:         runID,
		RepoID:        repoID,
		RepoTargetRef: "feature",
	}
	handler := pullMigRepoHandler(st)

	// Request without .git suffix
	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusOK)
}

func TestPullMigRepoHandler_MigNotFound(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()

	st := &runStore{
		getMigErr: pgx.ErrNoRows,
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPullMigRepoHandler_RepoNotInMig(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	migRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID},
		listMigReposByMigResult: []store.MigRepo{
			{
				ID:        migRepoID,
				MigID:     migID,
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
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPullMigRepoHandler_NoMatchingRun(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	migRepoID := domaintypes.NewMigRepoID()
	repoID := domaintypes.NewRepoID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID},
		listMigReposByMigResult: []store.MigRepo{
			{
				ID:        migRepoID,
				MigID:     migID,
				RepoID:    repoID,
				BaseRef:   "main",
				TargetRef: "feature",
			},
		},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://github.com/org/repo"},
		},
	}
	st.getLatestRunRepoByMigAndRepoStatus.err = pgx.ErrNoRows
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPullMigRepoHandler_InvalidMode(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()

	st := &runStore{
		getMigResult: store.Mig{ID: migID},
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo", "mode": "invalid"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestPullMigRepoHandler_MissingRepoURL(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()

	st := &runStore{}
	handler := pullMigRepoHandler(st)

	body := `{}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestPullMigRepoHandler_MissingMigID(t *testing.T) {
	t.Parallel()

	st := &runStore{}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs//pull", body, "mig_id", "")

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestPullMigRepoHandler_StoreError(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()

	st := &runStore{
		getMigResult:         store.Mig{ID: migID},
		listMigReposByMigErr: errors.New("database error"),
	}
	handler := pullMigRepoHandler(st)

	body := `{"repo_url": "https://github.com/org/repo"}`
	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+migID.String()+"/pull", body, "mig_id", migID.String())

	assertStatus(t, rr, http.StatusInternalServerError)
}
