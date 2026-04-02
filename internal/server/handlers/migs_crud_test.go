package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/migs — Create Mig
// =============================================================================

func TestMigs_Create(t *testing.T) {
	tests := []struct {
		name        string
		store       *migStore
		body        any
		wantStatus  int
		wantNoCalls bool
		verify      func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		{
			name:       "basic",
			store:      &migStore{},
			body:       map[string]any{"name": "my-mig"},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "CreateMig", st.createMigCalled)
				if st.createMigParams.Name != "my-mig" {
					t.Errorf("store Name = %q, want %q", st.createMigParams.Name, "my-mig")
				}
				resp := decodeBody[struct {
					ID        string  `json:"id"`
					Name      string  `json:"name"`
					SpecID    *string `json:"spec_id,omitempty"`
					CreatedAt string  `json:"created_at"`
				}](t, rr)
				if resp.Name != "my-mig" {
					t.Errorf("response Name = %q, want %q", resp.Name, "my-mig")
				}
				if resp.ID == "" {
					t.Error("response ID is empty")
				}
			},
		},
		{
			name:       "with spec",
			store:      &migStore{},
			body:       map[string]any{"name": "mig-with-spec", "spec": validSpecBody()},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "CreateMig", st.createMigCalled)
				assertCalled(t, "CreateSpec", st.createSpecCalled)
				assertCalled(t, "UpdateMigSpec", st.updateModSpec.called)
				resp := decodeBody[struct {
					ID     string  `json:"id"`
					Name   string  `json:"name"`
					SpecID *string `json:"spec_id,omitempty"`
				}](t, rr)
				if resp.SpecID == nil {
					t.Error("response spec_id is nil, expected non-nil when spec provided")
				}
			},
		},
		// Error paths
		{name: "empty name", store: &migStore{}, body: map[string]any{"name": ""}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "invalid name (spaces)", store: &migStore{}, body: map[string]any{"name": "my mig"}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "invalid JSON", store: &migStore{}, body: "not json", wantStatus: http.StatusBadRequest},
		{name: "invalid spec (legacy)", store: &migStore{}, body: map[string]any{
			"name": "mig-invalid-spec",
			"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}},
		}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "duplicate name", store: &migStore{createMigErr: &pgconn.PgError{Code: "23505"}}, body: map[string]any{"name": "existing-mig"}, wantStatus: http.StatusConflict},
		{name: "store error", store: &migStore{createMigErr: errors.New("database connection failed")}, body: map[string]any{"name": "test-mig"}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := createMigHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs", tt.body)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantNoCalls {
				assertNotCalled(t, "CreateMig", tt.store.createMigCalled)
			}
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

// =============================================================================
// GET /v1/migs — List Migs
// =============================================================================

func TestMigs_List(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		store        *migStore
		query        string
		wantStatus   int
		wantArchived *bool // when set, asserts listMigsParams.ArchivedOnly
		verify       func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "returns migs",
			store: &migStore{
				listMigsResult: []store.Mig{
					{ID: "mod001", Name: "alpha-mig", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
					{ID: "mod002", Name: "beta-mig", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}},
				},
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, _ *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				type migItem struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Archived bool   `json:"archived"`
				}
				resp := decodeBody[struct{ Migs []migItem }](t, rr)
				if len(resp.Migs) != 2 {
					t.Fatalf("got %d migs, want 2", len(resp.Migs))
				}
				if resp.Migs[0].Name != "alpha-mig" {
					t.Errorf("first mig Name = %q, want %q", resp.Migs[0].Name, "alpha-mig")
				}
			},
		},
		{
			name:       "respects limit/offset",
			store:      &migStore{},
			query:      "limit=10&offset=5",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ListMigs", st.listMigsCalled)
				if st.listMigsParams.Limit != 10 {
					t.Errorf("Limit = %d, want 10", st.listMigsParams.Limit)
				}
				if st.listMigsParams.Offset != 5 {
					t.Errorf("Offset = %d, want 5", st.listMigsParams.Offset)
				}
			},
		},
		{
			name:       "name_substring filter",
			store:      &migStore{},
			query:      "name_substring=alpha",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				if st.listMigsParams.NameFilter == nil {
					t.Fatal("NameFilter is nil, expected pointer to 'alpha'")
				}
				if *st.listMigsParams.NameFilter != "alpha" {
					t.Errorf("NameFilter = %q, want %q", *st.listMigsParams.NameFilter, "alpha")
				}
			},
		},
		{
			name:         "archived=true filter",
			store:        &migStore{},
			query:        "archived=true",
			wantStatus:   http.StatusOK,
			wantArchived: ptr(true),
		},
		{
			name:         "archived=false filter",
			store:        &migStore{},
			query:        "archived=false",
			wantStatus:   http.StatusOK,
			wantArchived: ptr(false),
		},
		{
			name: "repo_url filter normalizes",
			store: &migStore{
				listMigsResult: []store.Mig{
					{ID: "mod001", Name: "alpha", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
					{ID: "mod002", Name: "beta", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
				},
				listMigReposByModResults: map[string][]store.MigRepo{
					"mod001": {{ID: "repo1", MigID: "mod001", RepoID: "repo1"}},
					"mod002": {{ID: "repo2", MigID: "mod002", RepoID: "repo2"}},
				},
				repoByID: map[types.RepoID]store.Repo{
					"repo1": {ID: "repo1", Url: "https://github.com/org/repo"},
					"repo2": {ID: "repo2", Url: "https://github.com/org/other"},
				},
			},
			query:      "repo_url=https://github.com/org/repo.git/",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, _ *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				type migItem struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}
				resp := decodeBody[struct{ Migs []migItem }](t, rr)
				if len(resp.Migs) != 1 {
					t.Fatalf("got %d migs, want 1", len(resp.Migs))
				}
				if resp.Migs[0].ID != "mod001" {
					t.Errorf("id = %q, want %q", resp.Migs[0].ID, "mod001")
				}
			},
		},
		{
			name: "repo_url filter paginates",
			store: &migStore{
				listMigsResult: []store.Mig{
					{ID: "mod00A", Name: "a", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
					{ID: "mod00B", Name: "b", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
					{ID: "mod00C", Name: "c", CreatedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true}},
				},
				listMigReposByModResults: map[string][]store.MigRepo{
					"mod00A": {{ID: "repoA", MigID: "mod00A", RepoID: "repoA"}},
					"mod00B": {{ID: "repoB", MigID: "mod00B", RepoID: "repoB"}},
					"mod00C": {{ID: "repoC", MigID: "mod00C", RepoID: "repoC"}},
				},
				repoByID: map[types.RepoID]store.Repo{
					"repoA": {ID: "repoA", Url: "https://github.com/org/repo"},
					"repoB": {ID: "repoB", Url: "https://github.com/org/repo"},
					"repoC": {ID: "repoC", Url: "https://github.com/org/repo"},
				},
			},
			query:      "repo_url=https://github.com/org/repo&limit=1&offset=1",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, _ *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				type migItem struct {
					ID string `json:"id"`
				}
				resp := decodeBody[struct{ Migs []migItem }](t, rr)
				if len(resp.Migs) != 1 {
					t.Fatalf("got %d migs, want 1", len(resp.Migs))
				}
				if resp.Migs[0].ID != "mod00B" {
					t.Errorf("id = %q, want %q", resp.Migs[0].ID, "mod00B")
				}
			},
		},
		{
			name:       "store error",
			store:      &migStore{listMigsErr: errors.New("database connection failed")},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "invalid limit",
			store:      &migStore{},
			query:      "limit=notanumber",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid archived",
			store:      &migStore{},
			query:      "archived=notabool",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/v1/migs"
			if tt.query != "" {
				path += "?" + tt.query
			}
			rr := doRequest(t, listMigsHandler(tt.store), http.MethodGet, path, nil)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantArchived != nil {
				if tt.store.listMigsParams.ArchivedOnly == nil {
					t.Fatal("ArchivedOnly is nil")
				}
				if *tt.store.listMigsParams.ArchivedOnly != *tt.wantArchived {
					t.Errorf("ArchivedOnly = %v, want %v", *tt.store.listMigsParams.ArchivedOnly, *tt.wantArchived)
				}
			}
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

// =============================================================================
// DELETE /v1/migs/{mig_ref} — Delete Mig
// =============================================================================

func TestMigs_Delete(t *testing.T) {
	tests := []struct {
		name       string
		store      *migStore
		migRef     string
		wantStatus int
		verify     func(t *testing.T, st *migStore)
	}{
		{
			name:       "success",
			store:      &migStore{listRunsResult: []store.Run{}},
			migRef:     "mod123",
			wantStatus: http.StatusNoContent,
			verify: func(t *testing.T, st *migStore) {
				t.Helper()
				assertCalled(t, "GetMig", st.getModCalled)
				assertCalled(t, "DeleteMig", st.deleteMig.called)
				if st.deleteMig.params != "mod123" {
					t.Errorf("DeleteMig param = %q, want %q", st.deleteMig.params, "mod123")
				}
			},
		},
		{
			name:       "not found",
			store:      &migStore{getModErr: pgx.ErrNoRows},
			migRef:     "nonexistent",
			wantStatus: http.StatusNotFound,
			verify: func(t *testing.T, st *migStore) {
				t.Helper()
				assertNotCalled(t, "DeleteMig", st.deleteMig.called)
			},
		},
		{
			name: "refuses with runs",
			store: &migStore{
				listRunsResult: []store.Run{{ID: "run1", MigID: "mod123"}},
			},
			migRef:     "mod123",
			wantStatus: http.StatusConflict,
			verify: func(t *testing.T, st *migStore) {
				t.Helper()
				assertNotCalled(t, "DeleteMig", st.deleteMig.called)
			},
		},
		{
			name: "by name",
			store: &migStore{
				getModErr:          pgx.ErrNoRows,
				getModByNameResult: store.Mig{ID: "mod123", Name: "my-mig"},
				listRunsResult:     []store.Run{},
			},
			migRef:     "my-mig",
			wantStatus: http.StatusNoContent,
			verify: func(t *testing.T, st *migStore) {
				t.Helper()
				assertCalled(t, "GetMigByName", st.getModByNameCalled)
				if st.deleteMig.params != "mod123" {
					t.Errorf("DeleteMig param = %q, want %q", st.deleteMig.params, "mod123")
				}
			},
		},
		{
			name: "store error",
			store: func() *migStore {
				st := &migStore{listRunsResult: []store.Run{}}
				st.deleteMig.err = errors.New("database connection failed")
				return st
			}(),
			migRef:     "mod123",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := deleteMigHandler(tt.store)
			rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/"+tt.migRef, nil, "mig_ref", tt.migRef)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store)
			}
		})
	}
}
