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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/mods/{mod_ref}/specs — Set Mod Spec
// =============================================================================

// TestMods_SetSpec_Success verifies POST /v1/mods/{mod_ref}/specs creates a new spec and updates mods.spec_id.
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
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_SetSpec_WithName verifies POST /v1/mods/{mod_ref}/specs accepts optional name.
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
	req.SetPathValue("mod_ref", "mod123")
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
	req1.SetPathValue("mod_ref", "mod123")
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
	req2.SetPathValue("mod_ref", "mod123")
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

// TestMods_SetSpec_MissingSpec verifies POST /v1/mods/{mod_ref}/specs rejects missing spec.
func TestMods_SetSpec_MissingSpec(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	reqBody := map[string]any{"name": "no-spec"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_SetSpec_InvalidSpec verifies POST /v1/mods/{mod_ref}/specs rejects invalid spec JSON.
func TestMods_SetSpec_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	// Legacy spec shape with "mod" key is rejected.
	reqBody := map[string]any{
		"spec": map[string]any{"mod": map[string]any{"command": "echo hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader(body))
	req.SetPathValue("mod_ref", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestMods_SetSpec_ModNotFound verifies POST /v1/mods/{mod_ref}/specs returns 404 for missing mod.
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
	req.SetPathValue("mod_ref", "nonexistent")
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

// TestMods_SetSpec_ArchivedMod verifies POST /v1/mods/{mod_ref}/specs rejects archived mods.
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
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_SetSpec_InvalidJSON verifies POST /v1/mods/{mod_ref}/specs rejects malformed JSON body.
func TestMods_SetSpec_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := setModSpecHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/specs", bytes.NewReader([]byte("not json")))
	req.SetPathValue("mod_ref", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestMods_SetSpec_CreateSpecError verifies POST /v1/mods/{mod_ref}/specs returns 500 on CreateSpec failure.
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
	req.SetPathValue("mod_ref", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestMods_SetSpec_UpdateModSpecError verifies POST /v1/mods/{mod_ref}/specs returns 500 on UpdateModSpec failure.
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
	req.SetPathValue("mod_ref", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
