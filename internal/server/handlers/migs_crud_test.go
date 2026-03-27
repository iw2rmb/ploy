package handlers

import (
	"errors"
	"net/http"
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

// TestMods_Create_Success verifies POST /v1/migs creates a mig with valid input.
func TestMods_Create_Success(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs", map[string]any{"name": "my-mig"})
	assertStatus(t, rr, http.StatusCreated)

	if !st.createMigCalled {
		t.Error("store.CreateMig was not called")
	}
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
}

// TestMods_Create_WithSpec verifies POST /v1/migs with spec creates both mig and spec.
func TestMods_Create_WithSpec(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	reqBody := map[string]any{
		"name": "mig-with-spec",
		"spec": validSpecBody(),
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs", reqBody)
	assertStatus(t, rr, http.StatusCreated)

	if !st.createMigCalled {
		t.Error("store.CreateMig was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateMigSpec was not called")
	}

	resp := decodeBody[struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		SpecID *string `json:"spec_id,omitempty"`
	}](t, rr)
	if resp.SpecID == nil {
		t.Error("response spec_id is nil, expected non-nil when spec provided")
	}
}

// TestMods_Create_ErrorPaths verifies POST /v1/migs rejects invalid input and
// returns appropriate error status codes.
func TestMods_Create_ErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		store      *mockStore
		body       any
		wantStatus int
		wantNoCalls bool // if true, createMigCalled must be false
	}{
		{name: "empty name", store: &mockStore{}, body: map[string]any{"name": ""}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "invalid name (spaces)", store: &mockStore{}, body: map[string]any{"name": "my mig"}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "invalid JSON", store: &mockStore{}, body: "not json", wantStatus: http.StatusBadRequest},
		{name: "invalid spec (legacy)", store: &mockStore{}, body: map[string]any{
			"name": "mig-invalid-spec",
			"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}},
		}, wantStatus: http.StatusBadRequest, wantNoCalls: true},
		{name: "duplicate name", store: &mockStore{createMigErr: &pgconn.PgError{Code: "23505"}}, body: map[string]any{"name": "existing-mig"}, wantStatus: http.StatusConflict},
		{name: "store error", store: &mockStore{createMigErr: errors.New("database connection failed")}, body: map[string]any{"name": "test-mig"}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := createMigHandler(tt.store)
			rr := doJSON(t, handler, http.MethodPost, "/v1/migs", tt.body)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantNoCalls && tt.store.createMigCalled {
				t.Error("store.CreateMig should not be called")
			}
		})
	}
}

// =============================================================================
// GET /v1/migs — List Migs
// =============================================================================

// TestMods_List_Success verifies GET /v1/migs returns migs list.
func TestMods_List_Success(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listMigsResult: []store.Mig{
			{ID: "mod1", Name: "alpha-mig", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "mod2", Name: "beta-mig", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}},
		},
	}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs", nil)
	assertStatus(t, rr, http.StatusOK)

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
}

// TestMods_List_WithPagination verifies GET /v1/migs respects limit/offset.
func TestMods_List_WithPagination(t *testing.T) {
	st := &mockStore{}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs?limit=10&offset=5", nil)
	assertStatus(t, rr, http.StatusOK)

	if !st.listMigsCalled {
		t.Fatal("store.ListMigs was not called")
	}
	if st.listMigsParams.Limit != 10 {
		t.Errorf("Limit = %d, want 10", st.listMigsParams.Limit)
	}
	if st.listMigsParams.Offset != 5 {
		t.Errorf("Offset = %d, want 5", st.listMigsParams.Offset)
	}
}

// TestMods_List_WithNameFilter verifies GET /v1/migs respects name_substring filter.
func TestMods_List_WithNameFilter(t *testing.T) {
	st := &mockStore{}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs?name_substring=alpha", nil)
	assertStatus(t, rr, http.StatusOK)

	if st.listMigsParams.NameFilter == nil {
		t.Fatal("NameFilter is nil, expected pointer to 'alpha'")
	}
	if *st.listMigsParams.NameFilter != "alpha" {
		t.Errorf("NameFilter = %q, want %q", *st.listMigsParams.NameFilter, "alpha")
	}
}

// TestMods_List_ArchivedFilter verifies GET /v1/migs respects archived filter.
func TestMods_List_ArchivedFilter(t *testing.T) {
	tests := []struct {
		query        string
		wantArchived *bool
	}{
		{"archived=true", boolPtr(true)},
		{"archived=false", boolPtr(false)},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			st := &mockStore{}

			rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs?"+tt.query, nil)
			assertStatus(t, rr, http.StatusOK)

			if st.listMigsParams.ArchivedOnly == nil {
				t.Fatal("ArchivedOnly is nil")
			}
			if *st.listMigsParams.ArchivedOnly != *tt.wantArchived {
				t.Errorf("ArchivedOnly = %v, want %v", *st.listMigsParams.ArchivedOnly, *tt.wantArchived)
			}
		})
	}
}

// TestMods_List_InvalidQueryParams verifies GET /v1/migs rejects invalid query params.
func TestMods_List_InvalidQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "invalid limit", query: "limit=notanumber"},
		{name: "invalid archived", query: "archived=notabool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doJSON(t, listMigsHandler(&mockStore{}), http.MethodGet, "/v1/migs?"+tt.query, nil)
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

// TestMods_List_WithRepoURLFilter_Normalizes verifies GET /v1/migs repo_url filter
// uses types.NormalizeRepoURL for matching.
func TestMods_List_WithRepoURLFilter_Normalizes(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listMigsResult: []store.Mig{
			{ID: "mod1", Name: "alpha", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "mod2", Name: "beta", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
		},
		listMigReposByModResults: map[string][]store.MigRepo{
			"mod1": {{ID: "repo1", MigID: "mod1", RepoID: "repo1"}},
			"mod2": {{ID: "repo2", MigID: "mod2", RepoID: "repo2"}},
		},
		repoByID: map[types.RepoID]store.Repo{
			"repo1": {ID: "repo1", Url: "https://github.com/org/repo"},
			"repo2": {ID: "repo2", Url: "https://github.com/org/other"},
		},
	}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs?repo_url=https://github.com/org/repo.git/", nil)
	assertStatus(t, rr, http.StatusOK)

	type migItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	resp := decodeBody[struct{ Migs []migItem }](t, rr)
	if len(resp.Migs) != 1 {
		t.Fatalf("got %d migs, want 1", len(resp.Migs))
	}
	if resp.Migs[0].ID != "mod1" {
		t.Errorf("id = %q, want %q", resp.Migs[0].ID, "mod1")
	}
}

// TestMods_List_WithRepoURLFilter_Paginates verifies limit/offset apply after repo_url filtering.
func TestMods_List_WithRepoURLFilter_Paginates(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listMigsResult: []store.Mig{
			{ID: "modA", Name: "a", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "modB", Name: "b", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
			{ID: "modC", Name: "c", CreatedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true}},
		},
		listMigReposByModResults: map[string][]store.MigRepo{
			"modA": {{ID: "repoA", MigID: "modA", RepoID: "repoA"}},
			"modB": {{ID: "repoB", MigID: "modB", RepoID: "repoB"}},
			"modC": {{ID: "repoC", MigID: "modC", RepoID: "repoC"}},
		},
		repoByID: map[types.RepoID]store.Repo{
			"repoA": {ID: "repoA", Url: "https://github.com/org/repo"},
			"repoB": {ID: "repoB", Url: "https://github.com/org/repo"},
			"repoC": {ID: "repoC", Url: "https://github.com/org/repo"},
		},
	}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs?repo_url=https://github.com/org/repo&limit=1&offset=1", nil)
	assertStatus(t, rr, http.StatusOK)

	type migItem struct{ ID string `json:"id"` }
	resp := decodeBody[struct{ Migs []migItem }](t, rr)
	if len(resp.Migs) != 1 {
		t.Fatalf("got %d migs, want 1", len(resp.Migs))
	}
	if resp.Migs[0].ID != "modB" {
		t.Errorf("id = %q, want %q", resp.Migs[0].ID, "modB")
	}
}

// TestMods_List_StoreError verifies GET /v1/migs returns 500 on store error.
func TestMods_List_StoreError(t *testing.T) {
	st := &mockStore{listMigsErr: errors.New("database connection failed")}

	rr := doRequest(t, listMigsHandler(st), http.MethodGet, "/v1/migs", nil)
	assertStatus(t, rr, http.StatusInternalServerError)
}

// =============================================================================
// DELETE /v1/migs/{mig_ref} — Delete Mig
// =============================================================================

// TestMods_Delete_Success verifies DELETE /v1/migs/{mig_ref} deletes a mig.
// Tests mig deletion when no runs exist.
func TestMods_Delete_Success(t *testing.T) {
	st := &mockStore{
		// No runs exist for this mig.
		listRunsResult: []store.Run{},
	}
	handler := deleteMigHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/mod123", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusNoContent)

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.deleteMigCalled {
		t.Error("store.DeleteMig was not called")
	}
	if st.deleteMigParam != "mod123" {
		t.Errorf("DeleteMig param = %q, want %q", st.deleteMigParam, "mod123")
	}
}

// TestMods_Delete_NotFound verifies DELETE /v1/migs/{mig_ref} returns 404 for missing mig.
func TestMods_Delete_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := deleteMigHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/nonexistent", nil, "mig_ref", "nonexistent")

	assertStatus(t, rr, http.StatusNotFound)

	// DeleteMig should not be called.
	if st.deleteMigCalled {
		t.Error("store.DeleteMig should not be called for missing mig")
	}
}

// TestMods_Delete_RefusesWithRuns verifies DELETE /v1/migs/{mig_ref} returns 409
// when runs exist for the mig.
// Deletion is refused if any runs exist for the mig.
func TestMods_Delete_RefusesWithRuns(t *testing.T) {
	st := &mockStore{
		// Runs exist for this mig.
		listRunsResult: []store.Run{
			{ID: "run1", MigID: "mod123"},
		},
	}
	handler := deleteMigHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/mod123", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusConflict)

	// DeleteMig should not be called.
	if st.deleteMigCalled {
		t.Error("store.DeleteMig should not be called when runs exist")
	}
}

func TestMods_Delete_ByName(t *testing.T) {
	st := &mockStore{
		getModErr:          pgx.ErrNoRows,
		getModByNameResult: store.Mig{ID: "mod123", Name: "my-mig"},
		// No runs exist for this mig.
		listRunsResult: []store.Run{},
	}
	handler := deleteMigHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/my-mig", nil, "mig_ref", "my-mig")

	assertStatus(t, rr, http.StatusNoContent)
	if !st.getModByNameCalled {
		t.Error("store.GetMigByName was not called")
	}
	if st.deleteMigParam != "mod123" {
		t.Errorf("DeleteMig param = %q, want %q", st.deleteMigParam, "mod123")
	}
}

// TestMods_Delete_StoreError verifies DELETE /v1/migs/{mig_ref} returns 500 on store error.
func TestMods_Delete_StoreError(t *testing.T) {
	st := &mockStore{
		listRunsResult: []store.Run{}, // No runs.
		deleteMigErr:   errors.New("database connection failed"),
	}
	handler := deleteMigHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/mod123", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusInternalServerError)
}

