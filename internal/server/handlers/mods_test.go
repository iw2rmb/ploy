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
// uses vcs.NormalizeRepoURL for matching.
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

// =============================================================================
// DELETE /v1/mods/{mod_id} — Delete Mod
// =============================================================================

// TestMods_Delete_Success verifies DELETE /v1/mods/{mod_id} deletes a mod.
// Tests mod deletion when no runs exist.
func TestMods_Delete_Success(t *testing.T) {
	st := &mockStore{
		// No runs exist for this mod.
		listRunsResult: []store.Run{},
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/mod123", nil)
	req.SetPathValue("mod_id", "mod123")
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

// TestMods_Delete_NotFound verifies DELETE /v1/mods/{mod_id} returns 404 for missing mod.
func TestMods_Delete_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/nonexistent", nil)
	req.SetPathValue("mod_id", "nonexistent")
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

// TestMods_Delete_RefusesWithRuns verifies DELETE /v1/mods/{mod_id} returns 409
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
	req.SetPathValue("mod_id", "mod123")
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

// =============================================================================
// PATCH /v1/mods/{mod_id}/archive — Archive Mod
// =============================================================================

// TestMods_Archive_Success verifies PATCH /v1/mods/{mod_id}/archive archives a mod.
// Tests mod archiving to prevent execution.
func TestMods_Archive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
		// No runs/jobs for this mod.
		listRunsResult: []store.Run{},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMod was not called")
	}
	if !st.archiveModCalled {
		t.Error("store.ArchiveMod was not called")
	}
	if st.archiveModParam != "mod123" {
		t.Errorf("ArchiveMod param = %q, want %q", st.archiveModParam, "mod123")
	}

	// Verify response.
	var resp struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Archived {
		t.Error("response Archived = false, want true")
	}
}

// TestMods_Archive_AlreadyArchived verifies idempotent archive behavior.
func TestMods_Archive_AlreadyArchived(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Already archived.
		},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return OK (idempotent) without calling ArchiveMod again.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if st.archiveModCalled {
		t.Error("store.ArchiveMod should not be called for already-archived mod")
	}
}

// TestMods_Archive_NotFound verifies PATCH /v1/mods/{mod_id}/archive returns 404.
func TestMods_Archive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/nonexistent/archive", nil)
	req.SetPathValue("mod_id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestMods_Archive_RefusesWithRunningJobs verifies PATCH /v1/mods/{mod_id}/archive
// returns 409 when running jobs exist.
// Archive refuses if running jobs exist.
func TestMods_Archive_RefusesWithRunningJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		// A run exists for this mod.
		listRunsResult: []store.Run{
			{ID: "run1", ModID: "mod123"},
		},
		// That run has running jobs.
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusRunning},
		},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}

	// ArchiveMod should not be called.
	if st.archiveModCalled {
		t.Error("store.ArchiveMod should not be called when running jobs exist")
	}
}

// TestMods_Archive_RefusesWithQueuedJobs verifies PATCH /v1/mods/{mod_id}/archive
// returns 409 when queued jobs exist (also considered "running").
func TestMods_Archive_RefusesWithQueuedJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{
			{ID: "run1", ModID: "mod123"},
		},
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusQueued},
		},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

// TestMods_Archive_AllowsWithCompletedJobs verifies archive succeeds when
// only completed jobs exist.
func TestMods_Archive_AllowsWithCompletedJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{
			{ID: "run1", ModID: "mod123"},
		},
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusSuccess},
			{ID: "job2", RunID: "run1", Status: store.JobStatusFail},
		},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !st.archiveModCalled {
		t.Error("store.ArchiveMod should be called when only completed jobs exist")
	}
}

// =============================================================================
// PATCH /v1/mods/{mod_id}/unarchive — Unarchive Mod
// =============================================================================

// TestMods_Unarchive_Success verifies PATCH /v1/mods/{mod_id}/unarchive unarchives a mod.
// Tests mod unarchiving to allow execution again.
func TestMods_Unarchive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := unarchiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/unarchive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// Verify store methods called.
	if !st.unarchiveModCalled {
		t.Error("store.UnarchiveMod was not called")
	}
	if st.unarchiveModParam != "mod123" {
		t.Errorf("UnarchiveMod param = %q, want %q", st.unarchiveModParam, "mod123")
	}

	// Verify response.
	var resp struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Archived {
		t.Error("response Archived = true, want false")
	}
}

// TestMods_Unarchive_AlreadyUnarchived verifies idempotent unarchive behavior.
func TestMods_Unarchive_AlreadyUnarchived(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
	}
	handler := unarchiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/unarchive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return OK (idempotent) without calling UnarchiveMod.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if st.unarchiveModCalled {
		t.Error("store.UnarchiveMod should not be called for already-unarchived mod")
	}
}

// TestMods_Unarchive_NotFound verifies PATCH /v1/mods/{mod_id}/unarchive returns 404.
func TestMods_Unarchive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := unarchiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/nonexistent/unarchive", nil)
	req.SetPathValue("mod_id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// =============================================================================
// Error Handling
// =============================================================================

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

// TestMods_Delete_StoreError verifies DELETE /v1/mods/{mod_id} returns 500 on store error.
func TestMods_Delete_StoreError(t *testing.T) {
	st := &mockStore{
		listRunsResult: []store.Run{}, // No runs.
		deleteModErr:   errors.New("database connection failed"),
	}
	handler := deleteModHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/mods/mod123", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestMods_Archive_StoreError verifies PATCH /v1/mods/{mod_id}/archive returns 500 on store error.
func TestMods_Archive_StoreError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{},
		archiveModErr:  errors.New("database connection failed"),
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/archive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestMods_Unarchive_StoreError verifies PATCH /v1/mods/{mod_id}/unarchive returns 500 on store error.
func TestMods_Unarchive_StoreError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		unarchiveModErr: errors.New("database connection failed"),
	}
	handler := unarchiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/mod123/unarchive", nil)
	req.SetPathValue("mod_id", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// POST /v1/mods/{mod_id}/specs — Set Mod Spec
// =============================================================================

// TestMods_SetSpec_Success verifies POST /v1/mods/{mod_id}/specs creates a new spec and updates mods.spec_id.
// Tests append-only spec creation with mods.spec_id update.
func TestMods_SetSpec_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"spec": spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMod was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateModSpec was not called")
	}

	// Verify response shape.
	var resp struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
	if resp.CreatedAt == "" {
		t.Error("response created_at is empty")
	}
}

// TestMods_SetSpec_WithName verifies POST /v1/mods/{mod_id}/specs accepts optional name.
func TestMods_SetSpec_WithName(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"name": "my-named-spec",
		"spec": spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify name was passed to store.
	if st.createSpecParams.Name != "my-named-spec" {
		t.Errorf("spec name = %q, want %q", st.createSpecParams.Name, "my-named-spec")
	}
}

// TestMods_SetSpec_RepeatedCalls verifies that repeated set spec calls create new spec rows and update mods.spec_id.
// Scope: ROADMAP.md:313 — "repeated set spec creates new spec rows and changes mods.spec_id".
func TestMods_SetSpec_RepeatedCalls(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setModSpecHandler(st)

	// First call.
	spec1 := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody1 := map[string]any{"spec": spec1}
	body1, _ := json.Marshal(reqBody1)

	req1 := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body1))
	req1.SetPathValue("mod_id", "mod123")
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()

	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Fatalf("first call: status = %d, want %d", rr1.Code, http.StatusCreated)
	}

	var resp1 struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(rr1.Body).Decode(&resp1)
	firstSpecID := resp1.ID

	// Reset mock tracking for second call.
	st.createSpecCalled = false
	st.updateModSpecCalled = false

	// Second call with different spec.
	spec2 := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{"FOO": "bar"},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody2 := map[string]any{"spec": spec2}
	body2, _ := json.Marshal(reqBody2)

	req2 := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body2))
	req2.SetPathValue("mod_id", "mod123")
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()

	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusCreated {
		t.Fatalf("second call: status = %d, want %d", rr2.Code, http.StatusCreated)
	}

	// Verify second spec was created.
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called on second invocation")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateModSpec was not called on second invocation")
	}

	var resp2 struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(rr2.Body).Decode(&resp2)
	secondSpecID := resp2.ID

	// Spec IDs should differ (append-only).
	if firstSpecID == secondSpecID {
		t.Error("repeated calls should produce different spec IDs")
	}
}

// TestMods_SetSpec_MissingSpec verifies POST /v1/mods/{mod_id}/specs rejects missing spec.
func TestMods_SetSpec_MissingSpec(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	reqBody := map[string]any{"name": "no-spec"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Store should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called when spec is missing")
	}
}

// TestMods_SetSpec_InvalidSpec verifies POST /v1/mods/{mod_id}/specs rejects invalid spec JSON.
func TestMods_SetSpec_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	// Legacy spec shape with "mod" key is rejected.
	reqBody := map[string]any{
		"spec": map[string]any{"mod": map[string]any{"command": "echo hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestMods_SetSpec_ModNotFound verifies POST /v1/mods/{mod_id}/specs returns 404 for missing mod.
func TestMods_SetSpec_ModNotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{"spec": spec}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/nonexistent/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "nonexistent")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	// CreateSpec should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for missing mod")
	}
}

// TestMods_SetSpec_ArchivedMod verifies POST /v1/mods/{mod_id}/specs rejects archived mods.
func TestMods_SetSpec_ArchivedMod(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{"spec": spec}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}

	// CreateSpec should not be called for archived mods.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for archived mod")
	}
}

// TestMods_SetSpec_InvalidJSON verifies POST /v1/mods/{mod_id}/specs rejects malformed JSON body.
func TestMods_SetSpec_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader([]byte("not json")))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_SetSpec_CreateSpecError verifies POST /v1/mods/{mod_id}/specs returns 500 on CreateSpec failure.
func TestMods_SetSpec_CreateSpecError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		createSpecErr: errors.New("database connection failed"),
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{"spec": spec}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestMods_SetSpec_UpdateModSpecError verifies POST /v1/mods/{mod_id}/specs returns 500 on UpdateModSpec failure.
func TestMods_SetSpec_UpdateModSpecError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		updateModSpecErr: errors.New("database connection failed"),
	}
	handler := setModSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{"spec": spec}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
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
