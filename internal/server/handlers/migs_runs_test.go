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
				st.listMigReposByMigResult = []store.MigRepo{
					{ID: "repo1", MigID: "mig123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo2", MigID: "mig123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
				}
			},
			body:       allReposSelector(),
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMig", st.getMigCalled)
				assertCalled(t, "ListMigReposByMig", st.listMigReposByMigCalled)
				assertCalled(t, "CreateRun", st.createRunCalled)
				assertCalled(t, "CreateRunRepo", st.createRunRepoCalled)
				assertNotCalled(t, "CreateJob", st.createJobCalled)
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
				st.listMigReposByMigResult = []store.MigRepo{
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
				assertCalled(t, "CreateRunRepo", st.createRunRepoCalled)
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
				st.listMigReposByMigResult = []store.MigRepo{
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
				assertCalled(t, "ListMigReposByMig", st.listMigReposByMigCalled)
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
				if st.createRunParams.CreatedBy == nil || *st.createRunParams.CreatedBy != "test-user@example.com" {
					t.Errorf("created_by not propagated; got %v, want test-user@example.com", st.createRunParams.CreatedBy)
				}
			},
		},
		// ── Validation errors ────────────────────────────────────────────
		{name: "InvalidMode", store: &migStore{}, body: map[string]any{"repo_selector": map[string]any{"mode": "invalid"}}, wantStatus: http.StatusBadRequest},
		{name: "ExplicitEmptyRepos", store: &migStore{}, body: map[string]any{"repo_selector": map[string]any{"mode": "explicit", "repos": []string{}}}, wantStatus: http.StatusBadRequest},
		{name: "InvalidJSON", store: &migStore{}, body: "not json", wantStatus: http.StatusBadRequest},
		{name: "MigNotFound", store: &migStore{getMigErr: pgx.ErrNoRows}, body: allReposSelector(), wantStatus: http.StatusNotFound},
		{
			name: "ArchivedMig",
			store: func() *migStore {
				specID := domaintypes.SpecID("spec123")
				return &migStore{
					getMigResult: store.Mig{
						ID: "mig123", Name: "test-mig", SpecID: &specID,
						ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}
			}(),
			body: allReposSelector(), wantStatus: http.StatusConflict,
		},
		{
			name: "NoSpec",
			store: &migStore{
				getMigResult: store.Mig{ID: "mig123", Name: "test-mig", SpecID: nil, ArchivedAt: pgtype.Timestamptz{Valid: false}},
			},
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
		{name: "GetMigError", setupFn: func(st *migStore) { st.getMigErr = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "ListMigReposError", setupFn: func(st *migStore) { st.listMigReposByMigErr = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "CreateRunError", setupFn: func(st *migStore) { st.createRunErr = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
		{name: "CreateRunRepoError", setupFn: func(st *migStore) { st.createRunRepoErr = errors.New("database connection failed") }, body: allReposSelector(), wantStatus: http.StatusInternalServerError},
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
	assertNotCalled(t, "CreateRunRepo", st.createRunRepoCalled)
}
