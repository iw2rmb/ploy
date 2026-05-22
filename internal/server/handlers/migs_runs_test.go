package handlers

import (
	"bytes"
	"context"
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

// =============================================================================
// POST /v1/migs/{mig_id}/runs — Create Multi-Repo Run
// =============================================================================

func TestMigRuns_Create(t *testing.T) {
	tests := []struct {
		name       string
		store      *migStore       // use directly when set (overrides setupFn)
		setupFn    func(*migStore) // applied to activeMigWithSpec when store is nil
		body       any
		wantStatus int
		verify     func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		// ── Success paths ────────────────────────────────────────────────
		{
			name: "all repos",
			setupFn: func(st *migStore) {
				st.listMigReposByMig.val = []store.MigRepo{
					{ID: "migRepo1", MigID: "mig123", RepoID: "global01", BaseRef: "main", TargetRef: "feature1"},
					{ID: "migRepo2", MigID: "mig123", RepoID: "global02", BaseRef: "main", TargetRef: "feature2"},
				}
			},
			body:       allReposSelector(),
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMig", st.getMig.called)
				assertCalled(t, "ListMigReposByMig", st.listMigReposByMig.called)
				assertCalled(t, "CreateRun", st.createRun.called)
				assertCalled(t, "CreateRunRepo", st.createRunRepo.called)
				assertNotCalled(t, "CreateJob", st.createJob.called)
				if st.createRun.n != 1 {
					t.Fatalf("CreateRun calls = %d, want 1", st.createRun.n)
				}
				if len(st.createRunRepoParams) != 2 {
					t.Fatalf("CreateRunRepo calls = %d, want 2", len(st.createRunRepoParams))
				}
				if got := st.createRunRepoParams[0].RepoID; got != "global01" {
					t.Fatalf("first run_repo repo_id = %q, want global01", got)
				}
				if got := st.createRunRepoParams[1].RepoID; got != "global02" {
					t.Fatalf("second run_repo repo_id = %q, want global02", got)
				}
				for _, params := range st.createRunRepoParams {
					if params.RunID != st.createRun.params.ID {
						t.Fatalf("run_repo run_id = %q, want %q", params.RunID, st.createRun.params.ID)
					}
					if params.SourceCommitSha != testSourceCommitSHA || params.RepoSha0 != testSourceCommitSHA {
						t.Fatalf("run_repo SHA seed mismatch: source=%q sha0=%q", params.SourceCommitSha, params.RepoSha0)
					}
				}
				resp := decodeBody[struct {
					RunID string `json:"run_id"`
				}](t, rr)
				if resp.RunID == "" {
					t.Error("response run_id is empty")
				}
			},
		},
		{
			name: "failed repos",
			setupFn: func(st *migStore) {
				st.listMigReposByMig.val = []store.MigRepo{
					{ID: "repo1", MigID: "mig123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo2", MigID: "mig123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
					{ID: "repo3", MigID: "mig123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
				}
				st.listFailedRepoIDsByMig.val = []domaintypes.RepoID{"repo2"}
			},
			body:       map[string]any{"repo_selector": map[string]any{"mode": "failed"}},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ListFailedRepoIDsByMig", st.listFailedRepoIDsByMig.called)
				if st.listFailedRepoIDsByMig.params != "mig123" {
					t.Errorf("ListFailedRepoIDsByMig param = %q, want %q", st.listFailedRepoIDsByMig.params, "mig123")
				}
				assertCalled(t, "CreateRunRepo", st.createRunRepo.called)
				resp := decodeBody[struct {
					RunID string `json:"run_id"`
				}](t, rr)
				if resp.RunID == "" {
					t.Error("response run_id is empty")
				}
			},
		},
		{
			name: "explicit repos",
			setupFn: func(st *migStore) {
				st.listMigReposByMig.val = []store.MigRepo{
					{ID: "repo1", MigID: "mig123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo2", MigID: "mig123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
					{ID: "repo3", MigID: "mig123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
				}
				st.repoByID = map[domaintypes.RepoID]store.Repo{
					"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
					"repo2": {ID: "repo2", Url: "https://github.com/org/repo2"},
					"repo3": {ID: "repo3", Url: "https://github.com/org/repo3"},
				}
			},
			body: map[string]any{
				"repo_selector": map[string]any{
					"mode": "explicit",
					"repos": []string{
						"https://github.com/org/repo1.git",
						"https://github.com/org/repo3/",
					},
				},
			},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ListMigReposByMig", st.listMigReposByMig.called)
				resp := decodeBody[struct {
					RunID string `json:"run_id"`
				}](t, rr)
				if resp.RunID == "" {
					t.Error("response run_id is empty")
				}
			},
		},
		{
			name: "with created_by",
			body: func() map[string]any {
				b := allReposSelector()
				b["created_by"] = "test-user@example.com"
				return b
			}(),
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				if st.createRun.params.CreatedBy == nil || *st.createRun.params.CreatedBy != "test-user@example.com" {
					t.Errorf("created_by not propagated; got %v, want test-user@example.com", st.createRun.params.CreatedBy)
				}
			},
		},
		// ── Validation errors ────────────────────────────────────────────
		{name: "InvalidMode", store: &migStore{}, body: map[string]any{"repo_selector": map[string]any{"mode": "invalid"}}, wantStatus: http.StatusBadRequest},
		{name: "ExplicitEmptyRepos", store: &migStore{}, body: map[string]any{"repo_selector": map[string]any{"mode": "explicit", "repos": []string{}}}, wantStatus: http.StatusBadRequest},
		{name: "InvalidJSON", store: &migStore{}, body: "not json", wantStatus: http.StatusBadRequest},
		{name: "MigNotFound", store: func() *migStore { s := &migStore{}; s.getMig.err = pgx.ErrNoRows; return s }(), body: allReposSelector(), wantStatus: http.StatusNotFound},
		{
			name: "ArchivedMig",
			store: func() *migStore {
				specID := domaintypes.SpecID("spec123")
				s := &migStore{}
				s.getMig.val = store.Mig{
					ID: "mig123", Name: "test-mig", SpecID: &specID,
					ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}
				return s
			}(),
			body: allReposSelector(), wantStatus: http.StatusConflict,
		},
		{
			name: "NoSpec",
			store: func() *migStore {
				s := &migStore{}
				s.getMig.val = store.Mig{ID: "mig123", Name: "test-mig", SpecID: nil, ArchivedAt: pgtype.Timestamptz{Valid: false}}
				return s
			}(),
			body: allReposSelector(), wantStatus: http.StatusBadRequest,
		},
		{
			name: "NoReposSelected",
			store: func() *migStore {
				specID := domaintypes.SpecID("spec123")
				st := activeMigWithSpec(specID)
				st.listFailedRepoIDsByMig.val = []domaintypes.RepoID{}
				return st
			}(),
			body: map[string]any{"repo_selector": map[string]any{"mode": "failed"}}, wantStatus: http.StatusBadRequest,
		},
		// ── Store errors ─────────────────────────────────────────────────
		{name: "GetMigError", setupFn: func(st *migStore) { st.getMig.err = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "ListMigReposError", setupFn: func(st *migStore) { st.listMigReposByMig.err = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "CreateRunError", setupFn: func(st *migStore) { st.createRun.errs = []error{errors.New("database connection failed")} }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "CreateRunRepoError", setupFn: func(st *migStore) { st.createRunRepo.err = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "ListFailedReposError", setupFn: func(st *migStore) { st.listFailedRepoIDsByMig.err = errors.New("database connection failed") }, body: map[string]any{"repo_selector": map[string]any{"mode": "failed"}}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.store
			if st == nil {
				st = activeMigWithSpec(domaintypes.SpecID("spec123"))
				if tt.setupFn != nil {
					tt.setupFn(st)
				}
			}
			handler := createMigRunHandler(st)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mig123/runs", tt.body, "mig_id", "mig123")
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}

func TestMigRuns_Create_RejectsWhenSourceCommitSeedFails(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	st.repoByID = map[domaintypes.RepoID]store.Repo{
		"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
	}
	handler := createMigRunHandler(st)

	body, _ := json.Marshal(allReposSelector())

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mig123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mig123")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("seed lookup failed")
	}))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertNotCalled(t, "CreateRun", st.createRun.called)
	assertNotCalled(t, "CreateRunRepo", st.createRunRepo.called)
}
