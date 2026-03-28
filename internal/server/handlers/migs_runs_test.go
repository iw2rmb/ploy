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

// TestModRuns_Create_AllRepos verifies POST /v1/migs/{mig_id}/runs with mode="all"
// creates a run with all repos from the mig's repo set.
func TestModRuns_Create_AllRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	st.listMigReposByModResult = []store.MigRepo{
		{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
	}
	handler := createMigRunHandler(st)

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", allReposSelector(), "mig_id", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.listMigReposByModCalled {
		t.Error("store.ListMigReposByMig was not called")
	}
	if !st.createRunCalled {
		t.Error("store.CreateRun was not called")
	}
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}
	if st.createJobCalled {
		t.Error("store.CreateJob should not be called during run submission")
	}

	resp := decodeBody[struct{ RunID string `json:"run_id"` }](t, rr)
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_FailedRepos verifies POST /v1/migs/{mig_id}/runs with mode="failed"
// only selects repos whose last terminal status is 'Fail'.
func TestModRuns_Create_FailedRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	st.listMigReposByModResult = []store.MigRepo{
		{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
		{ID: "repo3", MigID: "mod123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
	}
	st.listFailedRepoIDsByMod.val = []domaintypes.RepoID{"repo2"}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{"mode": "failed"},
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", reqBody, "mig_id", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if !st.listFailedRepoIDsByMod.called {
		t.Error("store.ListFailedRepoIDsByMig was not called")
	}
	if st.listFailedRepoIDsByMod.params != "mod123" {
		t.Errorf("ListFailedRepoIDsByMig param = %q, want %q", st.listFailedRepoIDsByMod.params, "mod123")
	}
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}

	resp := decodeBody[struct{ RunID string `json:"run_id"` }](t, rr)
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_ExplicitRepos verifies POST /v1/migs/{mig_id}/runs with mode="explicit"
// only selects repos matching the provided repo URLs.
func TestModRuns_Create_ExplicitRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	st.listMigReposByModResult = []store.MigRepo{
		{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
		{ID: "repo3", MigID: "mod123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
	}
	st.repoByID = map[domaintypes.RepoID]store.Repo{
		"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
		"repo2": {ID: "repo2", Url: "https://github.com/org/repo2"},
		"repo3": {ID: "repo3", Url: "https://github.com/org/repo3"},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "explicit",
			"repos": []string{
				"https://github.com/org/repo1.git",
				"https://github.com/org/repo3/",
			},
		},
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", reqBody, "mig_id", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if !st.listMigReposByModCalled {
		t.Error("store.ListMigReposByMig was not called")
	}

	resp := decodeBody[struct{ RunID string `json:"run_id"` }](t, rr)
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_WithCreatedBy verifies POST /v1/migs/{mig_id}/runs passes created_by to store.
func TestModRuns_Create_WithCreatedBy(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	handler := createMigRunHandler(st)

	reqBody := allReposSelector()
	reqBody["created_by"] = "test-user@example.com"

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", reqBody, "mig_id", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if st.createRunParams.CreatedBy == nil || *st.createRunParams.CreatedBy != "test-user@example.com" {
		t.Errorf("created_by not propagated; got %v, want test-user@example.com", st.createRunParams.CreatedBy)
	}
}

// =============================================================================
// Validation Tests (table-driven)
// =============================================================================

// TestModRuns_Create_ValidationErrors merges individual validation error tests.
func TestModRuns_Create_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		store      *mockStore
		body       any
		wantStatus int
	}{
		{
			name:       "InvalidMode",
			store:      &mockStore{},
			body:       map[string]any{"repo_selector": map[string]any{"mode": "invalid"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ExplicitEmptyRepos",
			store:      &mockStore{},
			body:       map[string]any{"repo_selector": map[string]any{"mode": "explicit", "repos": []string{}}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ModNotFound",
			store:      &mockStore{getModErr: pgx.ErrNoRows},
			body:       allReposSelector(),
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ArchivedMod",
			store: func() *mockStore {
				specID := domaintypes.SpecID("spec123")
				return &mockStore{
					getModResult: store.Mig{
						ID:         "mod123",
						Name:       "test-mig",
						SpecID:     &specID,
						ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}
			}(),
			body:       allReposSelector(),
			wantStatus: http.StatusConflict,
		},
		{
			name: "NoSpec",
			store: &mockStore{
				getModResult: store.Mig{
					ID:         "mod123",
					Name:       "test-mig",
					SpecID:     nil,
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				},
			},
			body:       allReposSelector(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "NoReposSelected",
			store: func() *mockStore {
				specID := domaintypes.SpecID("spec123")
				st := activeMigWithSpec(specID)
				st.listFailedRepoIDsByMod.val = []domaintypes.RepoID{}
				return st
			}(),
			body:       map[string]any{"repo_selector": map[string]any{"mode": "failed"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidJSON",
			store:      &mockStore{},
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := createMigRunHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", tt.body, "mig_id", "mod123")
			assertStatus(t, rr, tt.wantStatus)
		})
	}
}

// =============================================================================
// Store Error Tests (table-driven)
// =============================================================================

// TestModRuns_Create_StoreErrors merges individual store error tests.
func TestModRuns_Create_StoreErrors(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(st *mockStore)
		body    any
	}{
		{
			name:    "GetMigError",
			setupFn: func(st *mockStore) { st.getModErr = errors.New("database connection failed") },
			body:    allReposSelector(),
		},
		{
			name: "ListModReposError",
			setupFn: func(st *mockStore) {
				st.listMigReposByModErr = errors.New("database connection failed")
			},
			body: allReposSelector(),
		},
		{
			name: "CreateRunError",
			setupFn: func(st *mockStore) {
				st.createRunErr = errors.New("database connection failed")
			},
			body: allReposSelector(),
		},
		{
			name: "CreateRunRepoError",
			setupFn: func(st *mockStore) {
				st.createRunRepoErr = errors.New("database connection failed")
			},
			body: allReposSelector(),
		},
		{
			name: "ListFailedReposError",
			setupFn: func(st *mockStore) {
				st.listFailedRepoIDsByMod.err = errors.New("database connection failed")
			},
			body: map[string]any{"repo_selector": map[string]any{"mode": "failed"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specID := domaintypes.SpecID("spec123")
			st := activeMigWithSpec(specID)
			tt.setupFn(st)

			handler := createMigRunHandler(st)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/runs", tt.body, "mig_id", "mod123")
			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}

func TestModRuns_Create_RejectsWhenSourceCommitSeedFails(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := activeMigWithSpec(specID)
	st.repoByID = map[domaintypes.RepoID]store.Repo{
		"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
	}
	handler := createMigRunHandler(st)

	body, _ := json.Marshal(allReposSelector())

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("seed lookup failed")
	}))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	if st.createRunRepoCalled {
		t.Fatal("store.CreateRunRepo should not be called when source commit seed resolution fails")
	}
}
