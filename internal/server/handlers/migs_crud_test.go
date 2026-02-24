package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/migs — Create Mig
// =============================================================================

// TestMods_Create_Success verifies POST /v1/migs creates a mig with valid input.
// Tests mig project creation endpoint.
func TestMods_Create_Success(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	reqBody := map[string]any{"name": "my-mig"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify store was called with correct params.
	if !st.createMigCalled {
		t.Error("store.CreateMig was not called")
	}
	if st.createMigParams.Name != "my-mig" {
		t.Errorf("store Name = %q, want %q", st.createMigParams.Name, "my-mig")
	}

	// Verify response shape.
	var resp struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		SpecID    *string `json:"spec_id,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "my-mig" {
		t.Errorf("response Name = %q, want %q", resp.Name, "my-mig")
	}
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
}

// TestMods_Create_WithSpec verifies POST /v1/migs with spec creates both mig and spec.
// Optional spec parameter creates initial spec row.
func TestMods_Create_WithSpec(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{
		"name": "mig-with-spec",
		"spec": spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify both mig and spec were created.
	if !st.createMigCalled {
		t.Error("store.CreateMig was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateMigSpec was not called")
	}

	// Verify response includes spec_id.
	var resp struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		SpecID *string `json:"spec_id,omitempty"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SpecID == nil {
		t.Error("response spec_id is nil, expected non-nil when spec provided")
	}
}

// TestMods_Create_EmptyName verifies POST /v1/migs rejects empty name.
func TestMods_Create_EmptyName(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	reqBody := map[string]any{"name": ""}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Store should not be called.
	if st.createMigCalled {
		t.Error("store.CreateMig should not be called for empty name")
	}
}

func TestMods_Create_InvalidName(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	reqBody := map[string]any{"name": "my mig"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if st.createMigCalled {
		t.Error("store.CreateMig should not be called for invalid name")
	}
}

// TestMods_Create_DuplicateName verifies POST /v1/migs returns 409 for duplicate name.
// Mig names must be unique.
func TestMods_Create_DuplicateName(t *testing.T) {
	// Simulate unique constraint violation (PostgreSQL error code 23505).
	st := &mockStore{
		createMigErr: &pgconn.PgError{Code: "23505"},
	}
	handler := createMigHandler(st)

	reqBody := map[string]any{"name": "existing-mig"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// TestMods_Create_InvalidJSON verifies POST /v1/migs rejects malformed JSON.
func TestMods_Create_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_Create_InvalidSpec verifies POST /v1/migs rejects invalid spec JSON.
// Legacy spec shapes (with top-level "mig" key) are rejected per
// internal/workflow/contracts/mods_spec.go:402-404.
func TestMods_Create_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := createMigHandler(st)

	// Legacy spec shape with "mig" key is explicitly rejected.
	reqBody := map[string]any{
		"name": "mig-invalid-spec",
		"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if st.createMigCalled {
		t.Error("store.CreateMig should not be called for invalid spec")
	}
}

// TestMods_Create_StoreError verifies POST /v1/migs returns 500 on store error.
func TestMods_Create_StoreError(t *testing.T) {
	st := &mockStore{
		createMigErr: errors.New("database connection failed"),
	}
	handler := createMigHandler(st)

	reqBody := map[string]any{"name": "test-mig"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// GET /v1/migs — List Migs
// =============================================================================

// TestMods_List_Success verifies GET /v1/migs returns migs list.
// Tests mig listing with pagination and filters.
func TestMods_List_Success(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listMigsResult: []store.Mig{
			{ID: "mod1", Name: "alpha-mig", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "mod2", Name: "beta-mig", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}},
		},
	}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Migs []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		} `json:"migs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

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
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?limit=10&offset=5", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify store received correct params.
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
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?name_substring=alpha", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify store received name filter.
	if st.listMigsParams.NameFilter == nil {
		t.Fatal("NameFilter is nil, expected pointer to 'alpha'")
	}
	if *st.listMigsParams.NameFilter != "alpha" {
		t.Errorf("NameFilter = %q, want %q", *st.listMigsParams.NameFilter, "alpha")
	}
}

// TestMods_List_ArchivedFilter verifies GET /v1/migs respects archived filter.
// Tests archived filter parameter.
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
			handler := listMigsHandler(st)

			req := httptest.NewRequest(http.MethodGet, "/v1/migs?"+tt.query, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}

			if st.listMigsParams.ArchivedOnly == nil {
				t.Fatal("ArchivedOnly is nil")
			}
			if *st.listMigsParams.ArchivedOnly != *tt.wantArchived {
				t.Errorf("ArchivedOnly = %v, want %v", *st.listMigsParams.ArchivedOnly, *tt.wantArchived)
			}
		})
	}
}

// TestMods_List_InvalidLimit verifies GET /v1/migs rejects invalid limit.
func TestMods_List_InvalidLimit(t *testing.T) {
	st := &mockStore{}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?limit=notanumber", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_List_InvalidArchived verifies GET /v1/migs rejects invalid archived value.
func TestMods_List_InvalidArchived(t *testing.T) {
	st := &mockStore{}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?archived=notabool", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
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
			"mod1": {{ID: "repo1", MigID: "mod1", RepoUrl: "https://github.com/org/repo"}},
			"mod2": {{ID: "repo2", MigID: "mod2", RepoUrl: "https://github.com/org/other"}},
		},
	}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?repo_url=https://github.com/org/repo.git/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Migs []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"migs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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
			"modA": {{ID: "repoA", MigID: "modA", RepoUrl: "https://github.com/org/repo"}},
			"modB": {{ID: "repoB", MigID: "modB", RepoUrl: "https://github.com/org/repo"}},
			"modC": {{ID: "repoC", MigID: "modC", RepoUrl: "https://github.com/org/repo"}},
		},
	}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs?repo_url=https://github.com/org/repo&limit=1&offset=1", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Migs []struct {
			ID string `json:"id"`
		} `json:"migs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Migs) != 1 {
		t.Fatalf("got %d migs, want 1", len(resp.Migs))
	}
	if resp.Migs[0].ID != "modB" {
		t.Errorf("id = %q, want %q", resp.Migs[0].ID, "modB")
	}
}

// TestMods_List_StoreError verifies GET /v1/migs returns 500 on store error.
func TestMods_List_StoreError(t *testing.T) {
	st := &mockStore{
		listMigsErr: errors.New("database connection failed"),
	}
	handler := listMigsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/migs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
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

	req := httptest.NewRequest(http.MethodDelete, "/v1/migs/mod123", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

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

	req := httptest.NewRequest(http.MethodDelete, "/v1/migs/nonexistent", nil)
	req.SetPathValue("mig_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

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

	req := httptest.NewRequest(http.MethodDelete, "/v1/migs/mod123", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}

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

	req := httptest.NewRequest(http.MethodDelete, "/v1/migs/my-mig", nil)
	req.SetPathValue("mig_ref", "my-mig")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
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

	req := httptest.NewRequest(http.MethodDelete, "/v1/migs/mod123", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func boolPtr(b bool) *bool {
	return &b
}
