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
// GET /v1/migs/{mig_ref}/specs/latest — Get Mig Latest Spec
// =============================================================================

func TestMods_GetLatestSpec_Success(t *testing.T) {
	specID := store.Spec{ID: "spec123", Spec: []byte(`{"version":"0.2.0","steps":[{"image":"alpine"}]}`)}
	migSpecID := specID.ID
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &migSpecID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: specID,
	}
	handler := getMigLatestSpecHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/migs/mod123/specs/latest", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusOK)
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
	if rr.Body.String() != string(specID.Spec) {
		t.Fatalf("body = %q, want %q", rr.Body.String(), string(specID.Spec))
	}
	if !st.getModCalled {
		t.Fatal("expected GetMig/GetMigByName to be called")
	}
	if !st.getSpecCalled {
		t.Fatal("expected GetSpec to be called")
	}
}

func TestMods_GetLatestSpec_MigWithoutSpec(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     nil,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := getMigLatestSpecHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/migs/mod123/specs/latest", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusNotFound)
	if st.getSpecCalled {
		t.Fatal("GetSpec must not be called when mig has no spec")
	}
}

func TestMods_GetLatestSpec_MigNotFound(t *testing.T) {
	st := &mockStore{getModErr: pgx.ErrNoRows}
	handler := getMigLatestSpecHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/migs/missing/specs/latest", nil, "mig_ref", "missing")

	assertStatus(t, rr, http.StatusNotFound)
	if st.getSpecCalled {
		t.Fatal("GetSpec must not be called when mig is missing")
	}
}

// =============================================================================
// POST /v1/migs/{mig_ref}/specs — Set Mig Spec
// =============================================================================

// TestMods_SetSpec_Success verifies POST /v1/migs/{mig_ref}/specs creates a new spec and updates migs.spec_id.
// Tests append-only spec creation with migs.spec_id update.
func TestMods_SetSpec_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{
		"spec": spec,
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusCreated)

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpecCalled {
		t.Error("store.UpdateMigSpec was not called")
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

// TestMods_SetSpec_WithName verifies POST /v1/migs/{mig_ref}/specs accepts optional name.
func TestMods_SetSpec_WithName(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{
		"name": "my-named-spec",
		"spec": spec,
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusCreated)

	// Verify name was passed to store.
	if st.createSpecParams.Name != "my-named-spec" {
		t.Errorf("spec name = %q, want %q", st.createSpecParams.Name, "my-named-spec")
	}
}

// TestMods_SetSpec_RepeatedCalls verifies that repeated set spec calls create new spec rows and update migs.spec_id.
func TestMods_SetSpec_RepeatedCalls(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	// First call.
	spec1 := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody1 := map[string]any{"spec": spec1}
	body1, _ := json.Marshal(reqBody1)

	req1 := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/specs", bytes.NewReader(body1))
	req1.SetPathValue("mig_ref", "mod123")
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
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody2 := map[string]any{"spec": spec2}
	body2, _ := json.Marshal(reqBody2)

	req2 := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/specs", bytes.NewReader(body2))
	req2.SetPathValue("mig_ref", "mod123")
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
		t.Error("store.UpdateMigSpec was not called on second invocation")
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

// TestMods_SetSpec_MissingSpec verifies POST /v1/migs/{mig_ref}/specs rejects missing spec.
func TestMods_SetSpec_MissingSpec(t *testing.T) {
	st := &mockStore{}
	handler := setMigSpecHandler(st)

	reqBody := map[string]any{"name": "no-spec"}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusBadRequest)

	// Store should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called when spec is missing")
	}
}

// TestMods_SetSpec_InvalidSpec verifies POST /v1/migs/{mig_ref}/specs rejects invalid spec JSON.
func TestMods_SetSpec_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := setMigSpecHandler(st)

	// Legacy spec shape with "mig" key is rejected.
	reqBody := map[string]any{
		"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}},
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestMods_SetSpec_ModNotFound verifies POST /v1/migs/{mig_ref}/specs returns 404 for missing mig.
func TestMods_SetSpec_ModNotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{"spec": spec}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/nonexistent/specs", reqBody, "mig_ref", "nonexistent")

	assertStatus(t, rr, http.StatusNotFound)

	// CreateSpec should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for missing mig")
	}
}

// TestMods_SetSpec_ArchivedMod verifies POST /v1/migs/{mig_ref}/specs rejects archived migs.
func TestMods_SetSpec_ArchivedMod(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{"spec": spec}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusConflict)

	// CreateSpec should not be called for archived migs.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for archived mig")
	}
}

// TestMods_SetSpec_InvalidJSON verifies POST /v1/migs/{mig_ref}/specs rejects malformed JSON body.
func TestMods_SetSpec_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := setMigSpecHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/specs", bytes.NewReader([]byte("not json")))
	req.SetPathValue("mig_ref", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestMods_SetSpec_CreateSpecError verifies POST /v1/migs/{mig_ref}/specs returns 500 on CreateSpec failure.
func TestMods_SetSpec_CreateSpecError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		createSpecErr: errors.New("database connection failed"),
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{"spec": spec}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusInternalServerError)
}

// TestMods_SetSpec_UpdateMigSpecError verifies POST /v1/migs/{mig_ref}/specs returns 500 on UpdateMigSpec failure.
func TestMods_SetSpec_UpdateMigSpecError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		updateModSpecErr: errors.New("database connection failed"),
	}
	handler := setMigSpecHandler(st)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
	reqBody := map[string]any{"spec": spec}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusInternalServerError)
}
