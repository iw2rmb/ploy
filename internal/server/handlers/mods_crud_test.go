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
// POST /v1/mods — Create Mod
// =============================================================================

// TestMods_Create_Success verifies POST /v1/mods creates a mod with valid input.
// Tests mod project creation endpoint.
func TestMods_Create_Success(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	reqBody := map[string]any{"name": "my-mod"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify store was called with correct params.
	if !st.createModCalled {
		t.Error("store.CreateMod was not called")
	}
	if st.createModParams.Name != "my-mod" {
		t.Errorf("store Name = %q, want %q", st.createModParams.Name, "my-mod")
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
	if resp.Name != "my-mod" {
		t.Errorf("response Name = %q, want %q", resp.Name, "my-mod")
	}
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
}

// TestMods_Create_WithSpec verifies POST /v1/mods with spec creates both mod and spec.
// Optional spec parameter creates initial spec row.
func TestMods_Create_WithSpec(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"name": "mod-with-spec",
		"spec": spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify both mod and spec were created.
	if !st.createModCalled {
		t.Error("store.CreateMod was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateModSpec was not called")
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

// TestMods_Create_EmptyName verifies POST /v1/mods rejects empty name.
func TestMods_Create_EmptyName(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	reqBody := map[string]any{"name": ""}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Store should not be called.
	if st.createModCalled {
		t.Error("store.CreateMod should not be called for empty name")
	}
}

func TestMods_Create_InvalidName(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	reqBody := map[string]any{"name": "my mod"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if st.createModCalled {
		t.Error("store.CreateMod should not be called for invalid name")
	}
}

// TestMods_Create_DuplicateName verifies POST /v1/mods returns 409 for duplicate name.
// Mod names must be unique.
func TestMods_Create_DuplicateName(t *testing.T) {
	// Simulate unique constraint violation (PostgreSQL error code 23505).
	st := &mockStore{
		createModErr: &pgconn.PgError{Code: "23505"},
	}
	handler := createModHandler(st)

	reqBody := map[string]any{"name": "existing-mod"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// TestMods_Create_InvalidJSON verifies POST /v1/mods rejects malformed JSON.
func TestMods_Create_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_Create_InvalidSpec verifies POST /v1/mods rejects invalid spec JSON.
// Legacy spec shapes (with top-level "mod" key) are rejected per
// internal/workflow/contracts/mods_spec.go:402-404.
func TestMods_Create_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	// Legacy spec shape with "mod" key is explicitly rejected.
	reqBody := map[string]any{
		"name": "mod-invalid-spec",
		"spec": map[string]any{"mod": map[string]any{"command": "echo hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if st.createModCalled {
		t.Error("store.CreateMod should not be called for invalid spec")
	}
}

// TestMods_Create_StoreError verifies POST /v1/mods returns 500 on store error.
func TestMods_Create_StoreError(t *testing.T) {
	st := &mockStore{
		createModErr: errors.New("database connection failed"),
	}
	handler := createModHandler(st)

	reqBody := map[string]any{"name": "test-mod"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// GET /v1/mods — List Mods
// =============================================================================

// TestMods_List_Success verifies GET /v1/mods returns mods list.
// Tests mod listing with pagination and filters.
func TestMods_List_Success(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listModsResult: []store.Mod{
			{ID: "mod1", Name: "alpha-mod", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "mod2", Name: "beta-mod", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}},
		},
	}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Mods []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Mods) != 2 {
		t.Fatalf("got %d mods, want 2", len(resp.Mods))
	}
	if resp.Mods[0].Name != "alpha-mod" {
		t.Errorf("first mod Name = %q, want %q", resp.Mods[0].Name, "alpha-mod")
	}
}

// TestMods_List_WithPagination verifies GET /v1/mods respects limit/offset.
func TestMods_List_WithPagination(t *testing.T) {
	st := &mockStore{}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?limit=10&offset=5", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify store received correct params.
	if !st.listModsCalled {
		t.Fatal("store.ListMods was not called")
	}
	if st.listModsParams.Limit != 10 {
		t.Errorf("Limit = %d, want 10", st.listModsParams.Limit)
	}
	if st.listModsParams.Offset != 5 {
		t.Errorf("Offset = %d, want 5", st.listModsParams.Offset)
	}
}

// TestMods_List_WithNameFilter verifies GET /v1/mods respects name_substring filter.
func TestMods_List_WithNameFilter(t *testing.T) {
	st := &mockStore{}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?name_substring=alpha", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify store received name filter.
	if st.listModsParams.NameFilter == nil {
		t.Fatal("NameFilter is nil, expected pointer to 'alpha'")
	}
	if *st.listModsParams.NameFilter != "alpha" {
		t.Errorf("NameFilter = %q, want %q", *st.listModsParams.NameFilter, "alpha")
	}
}

// TestMods_List_ArchivedFilter verifies GET /v1/mods respects archived filter.
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
			handler := listModsHandler(st)

			req := httptest.NewRequest(http.MethodGet, "/v1/mods?"+tt.query, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}

			if st.listModsParams.ArchivedOnly == nil {
				t.Fatal("ArchivedOnly is nil")
			}
			if *st.listModsParams.ArchivedOnly != *tt.wantArchived {
				t.Errorf("ArchivedOnly = %v, want %v", *st.listModsParams.ArchivedOnly, *tt.wantArchived)
			}
		})
	}
}

// TestMods_List_InvalidLimit verifies GET /v1/mods rejects invalid limit.
func TestMods_List_InvalidLimit(t *testing.T) {
	st := &mockStore{}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?limit=notanumber", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_List_InvalidArchived verifies GET /v1/mods rejects invalid archived value.
func TestMods_List_InvalidArchived(t *testing.T) {
	st := &mockStore{}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?archived=notabool", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_List_WithRepoURLFilter_Normalizes verifies GET /v1/mods repo_url filter
// uses types.NormalizeRepoURL for matching.
func TestMods_List_WithRepoURLFilter_Normalizes(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listModsResult: []store.Mod{
			{ID: "mod1", Name: "alpha", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "mod2", Name: "beta", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
		},
		listModReposByModResults: map[string][]store.ModRepo{
			"mod1": {{ID: "repo1", ModID: "mod1", RepoUrl: "https://github.com/org/repo"}},
			"mod2": {{ID: "repo2", ModID: "mod2", RepoUrl: "https://github.com/org/other"}},
		},
	}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?repo_url=https://github.com/org/repo.git/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Mods []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Mods) != 1 {
		t.Fatalf("got %d mods, want 1", len(resp.Mods))
	}
	if resp.Mods[0].ID != "mod1" {
		t.Errorf("id = %q, want %q", resp.Mods[0].ID, "mod1")
	}
}

// TestMods_List_WithRepoURLFilter_Paginates verifies limit/offset apply after repo_url filtering.
func TestMods_List_WithRepoURLFilter_Paginates(t *testing.T) {
	now := time.Now()
	st := &mockStore{
		listModsResult: []store.Mod{
			{ID: "modA", Name: "a", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
			{ID: "modB", Name: "b", CreatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}},
			{ID: "modC", Name: "c", CreatedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true}},
		},
		listModReposByModResults: map[string][]store.ModRepo{
			"modA": {{ID: "repoA", ModID: "modA", RepoUrl: "https://github.com/org/repo"}},
			"modB": {{ID: "repoB", ModID: "modB", RepoUrl: "https://github.com/org/repo"}},
			"modC": {{ID: "repoC", ModID: "modC", RepoUrl: "https://github.com/org/repo"}},
		},
	}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods?repo_url=https://github.com/org/repo&limit=1&offset=1", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Mods []struct {
			ID string `json:"id"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Mods) != 1 {
		t.Fatalf("got %d mods, want 1", len(resp.Mods))
	}
	if resp.Mods[0].ID != "modB" {
		t.Errorf("id = %q, want %q", resp.Mods[0].ID, "modB")
	}
}

// TestMods_List_StoreError verifies GET /v1/mods returns 500 on store error.
func TestMods_List_StoreError(t *testing.T) {
	st := &mockStore{
		listModsErr: errors.New("database connection failed"),
	}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// DELETE /v1/mods/{mod_ref} — Delete Mod
// =============================================================================

// TestMods_Delete_Success verifies DELETE /v1/mods/{mod_ref} deletes a mod.
// Tests mod deletion when no runs exist.
func TestMods_Delete_Success(t *testing.T) {
	st := &mockStore{
		// No runs exist for this mod.
		listRunsResult: []store.Run{},
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/mod123", nil)
	req.SetPathValue("mod_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMod was not called")
	}
	if !st.deleteModCalled {
		t.Error("store.DeleteMod was not called")
	}
	if st.deleteModParam != "mod123" {
		t.Errorf("DeleteMod param = %q, want %q", st.deleteModParam, "mod123")
	}
}

// TestMods_Delete_NotFound verifies DELETE /v1/mods/{mod_ref} returns 404 for missing mod.
func TestMods_Delete_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/nonexistent", nil)
	req.SetPathValue("mod_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	// DeleteMod should not be called.
	if st.deleteModCalled {
		t.Error("store.DeleteMod should not be called for missing mod")
	}
}

// TestMods_Delete_RefusesWithRuns verifies DELETE /v1/mods/{mod_ref} returns 409
// when runs exist for the mod.
// Deletion is refused if any runs exist for the mod.
func TestMods_Delete_RefusesWithRuns(t *testing.T) {
	st := &mockStore{
		// Runs exist for this mod.
		listRunsResult: []store.Run{
			{ID: "run1", ModID: "mod123"},
		},
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/mod123", nil)
	req.SetPathValue("mod_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}

	// DeleteMod should not be called.
	if st.deleteModCalled {
		t.Error("store.DeleteMod should not be called when runs exist")
	}
}

func TestMods_Delete_ByName(t *testing.T) {
	st := &mockStore{
		getModErr:          pgx.ErrNoRows,
		getModByNameResult: store.Mod{ID: "mod123", Name: "my-mod"},
		// No runs exist for this mod.
		listRunsResult: []store.Run{},
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/my-mod", nil)
	req.SetPathValue("mod_ref", "my-mod")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if !st.getModByNameCalled {
		t.Error("store.GetModByName was not called")
	}
	if st.deleteModParam != "mod123" {
		t.Errorf("DeleteMod param = %q, want %q", st.deleteModParam, "mod123")
	}
}

// TestMods_Delete_StoreError verifies DELETE /v1/mods/{mod_ref} returns 500 on store error.
func TestMods_Delete_StoreError(t *testing.T) {
	st := &mockStore{
		listRunsResult: []store.Run{}, // No runs.
		deleteModErr:   errors.New("database connection failed"),
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/mod123", nil)
	req.SetPathValue("mod_ref", "mod123")
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
