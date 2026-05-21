package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestPullRunRepoHandler(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	tests := []struct {
		name       string
		pathRunID  string
		body       string
		setup      func(*runStore)
		wantStatus int
		verify     func(*testing.T, *runStore, *httptest.ResponseRecorder)
	}{
		{
			name:      "success",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID, MigID: domaintypes.NewMigID()}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "feature-branch",
					RepoUrl:       "https://github.com/org/repo.git",
				}}
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *runStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				var resp pullResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}
				if resp.RunID != runID {
					t.Fatalf("run_id = %q, want %q", resp.RunID, runID)
				}
				if resp.RepoID != repoID {
					t.Fatalf("repo_id = %q, want %q", resp.RepoID, repoID)
				}
				if resp.RepoTargetRef != "feature-branch" {
					t.Fatalf("repo_target_ref = %q, want feature-branch", resp.RepoTargetRef)
				}
				assertCalled(t, "GetRun", st.getRun.called)
				assertCalled(t, "ListRunReposWithURLByRun", st.listRunReposWithURLByRun.called)
				if st.listRunReposWithURLByRun.params != runID.String() {
					t.Fatalf("ListRunReposWithURLByRun run_id = %q, want %q", st.listRunReposWithURLByRun.params, runID)
				}
			},
		},
		{
			name:      "git suffix normalization",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo.git"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "main",
					RepoUrl:       "https://github.com/org/repo",
				}}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "trailing slash normalization",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "main",
					RepoUrl:       "https://github.com/org/repo/",
				}}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "run not found",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getRun.err = pgx.ErrNoRows
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "repo not found",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/missing"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "main",
					RepoUrl:       "https://github.com/org/repo",
				}}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "multiple normalized matches",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
					{RunID: runID, RepoID: domaintypes.NewRepoID(), RepoTargetRef: "main", RepoUrl: "https://github.com/org/repo"},
					{RunID: runID, RepoID: domaintypes.NewRepoID(), RepoTargetRef: "develop", RepoUrl: "https://github.com/org/repo.git"},
				}
			},
			wantStatus: http.StatusConflict,
		},
		{name: "missing repo url", pathRunID: runID.String(), body: `{}`, setup: func(*runStore) {}, wantStatus: http.StatusBadRequest},
		{name: "missing run id", pathRunID: "", body: `{"repo_url":"https://github.com/org/repo"}`, setup: func(*runStore) {}, wantStatus: http.StatusBadRequest},
		{
			name:      "store error",
			pathRunID: runID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getRun.val = store.Run{ID: runID}
				st.listRunReposWithURLByRun.err = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &runStore{}
			tt.setup(st)
			rr := doRequest(t, pullRunRepoHandler(st), http.MethodPost, "/v1/runs/"+tt.pathRunID+"/pull", tt.body, "run_id", tt.pathRunID)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}

func TestPullMigRepoHandler(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	repoID := domaintypes.NewRepoID()
	runID := domaintypes.NewRunID()

	tests := []struct {
		name       string
		pathMigID  string
		body       string
		setup      func(*runStore)
		wantStatus int
		wantFilter domaintypes.RunRepoStatus
		verify     func(*testing.T, *runStore, *httptest.ResponseRecorder)
	}{
		{
			name:       "default last succeeded",
			pathMigID:  migID.String(),
			body:       `{"repo_url":"https://github.com/org/repo"}`,
			wantStatus: http.StatusOK,
			wantFilter: domaintypes.RunRepoStatusSuccess,
			setup: func(st *runStore) {
				setupMigPullRepo(st, migID, repoID, "https://github.com/org/repo")
				st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "feature-branch",
				}
			},
			verify: func(t *testing.T, _ *runStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				var resp pullResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}
				if resp.RunID != runID {
					t.Fatalf("run_id = %q, want %q", resp.RunID, runID)
				}
				if resp.RepoID != repoID {
					t.Fatalf("repo_id = %q, want %q", resp.RepoID, repoID)
				}
				if resp.RepoTargetRef != "feature-branch" {
					t.Fatalf("repo_target_ref = %q, want feature-branch", resp.RepoTargetRef)
				}
			},
		},
		{
			name:       "last failed",
			pathMigID:  migID.String(),
			body:       `{"repo_url":"https://github.com/org/repo","mode":"last-failed"}`,
			wantStatus: http.StatusOK,
			wantFilter: domaintypes.RunRepoStatusFail,
			setup: func(st *runStore) {
				setupMigPullRepo(st, migID, repoID, "https://github.com/org/repo")
				st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{
					RunID:         runID,
					RepoID:        repoID,
					RepoTargetRef: "bugfix-branch",
				}
			},
		},
		{
			name:       "git suffix normalization",
			pathMigID:  migID.String(),
			body:       `{"repo_url":"https://github.com/org/repo"}`,
			wantStatus: http.StatusOK,
			wantFilter: domaintypes.RunRepoStatusSuccess,
			setup: func(st *runStore) {
				setupMigPullRepo(st, migID, repoID, "https://github.com/org/repo.git")
				st.getLatestRunRepoByMigAndRepoStatus.val = store.GetLatestRunRepoByMigAndRepoStatusRow{RunID: runID, RepoID: repoID}
			},
		},
		{name: "mig not found", pathMigID: migID.String(), body: `{"repo_url":"https://github.com/org/repo"}`, setup: func(st *runStore) { st.getMig.err = pgx.ErrNoRows }, wantStatus: http.StatusNotFound},
		{
			name:      "repo not in mig",
			pathMigID: migID.String(),
			body:      `{"repo_url":"https://github.com/org/missing"}`,
			setup: func(st *runStore) {
				setupMigPullRepo(st, migID, repoID, "https://github.com/org/repo")
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "no matching run",
			pathMigID: migID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				setupMigPullRepo(st, migID, repoID, "https://github.com/org/repo")
				st.getLatestRunRepoByMigAndRepoStatus.err = pgx.ErrNoRows
			},
			wantStatus: http.StatusNotFound,
			wantFilter: domaintypes.RunRepoStatusSuccess,
		},
		{name: "invalid mode", pathMigID: migID.String(), body: `{"repo_url":"https://github.com/org/repo","mode":"invalid"}`, setup: func(st *runStore) { st.getMig.val = store.Mig{ID: migID} }, wantStatus: http.StatusBadRequest},
		{name: "missing repo url", pathMigID: migID.String(), body: `{}`, setup: func(*runStore) {}, wantStatus: http.StatusBadRequest},
		{name: "missing mig id", pathMigID: "", body: `{"repo_url":"https://github.com/org/repo"}`, setup: func(*runStore) {}, wantStatus: http.StatusBadRequest},
		{
			name:      "store error",
			pathMigID: migID.String(),
			body:      `{"repo_url":"https://github.com/org/repo"}`,
			setup: func(st *runStore) {
				st.getMig.val = store.Mig{ID: migID}
				st.listMigReposByMig.err = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &runStore{}
			tt.setup(st)
			rr := doRequest(t, pullMigRepoHandler(st), http.MethodPost, "/v1/migs/"+tt.pathMigID+"/pull", tt.body, "mig_id", tt.pathMigID)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantFilter != "" && st.getLatestRunRepoByMigAndRepoStatus.params.Status != tt.wantFilter {
				t.Fatalf("status filter = %q, want %q", st.getLatestRunRepoByMigAndRepoStatus.params.Status, tt.wantFilter)
			}
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}

func setupMigPullRepo(st *runStore, migID domaintypes.MigID, repoID domaintypes.RepoID, repoURL string) {
	st.getMig.val = store.Mig{ID: migID, Name: "test-mig"}
	st.listMigReposByMig.val = []store.MigRepo{{
		ID:        domaintypes.NewMigRepoID(),
		MigID:     migID,
		RepoID:    repoID,
		BaseRef:   "main",
		TargetRef: "feature",
	}}
	st.repoByID = map[domaintypes.RepoID]store.Repo{
		repoID: {ID: repoID, Url: repoURL},
	}
}
