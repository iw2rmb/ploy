package handlers

import (
	"errors"
	"net/http"
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
	st := &migStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &migSpecID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	st.getSpec.val = specID
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
	if !st.getSpec.called {
		t.Fatal("expected GetSpec to be called")
	}
}

func TestMods_GetLatestSpec_MigWithoutSpec(t *testing.T) {
	st := &migStore{
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
	if st.getSpec.called {
		t.Fatal("GetSpec must not be called when mig has no spec")
	}
}

func TestMods_GetLatestSpec_MigNotFound(t *testing.T) {
	st := &migStore{getModErr: pgx.ErrNoRows}
	handler := getMigLatestSpecHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/migs/missing/specs/latest", nil, "mig_ref", "missing")

	assertStatus(t, rr, http.StatusNotFound)
	if st.getSpec.called {
		t.Fatal("GetSpec must not be called when mig is missing")
	}
}

// =============================================================================
// POST /v1/migs/{mig_ref}/specs — Set Mig Spec
// =============================================================================

// TestMods_SetSpec_Success verifies POST /v1/migs/{mig_ref}/specs creates a new spec and updates migs.spec_id.
func TestMods_SetSpec_Success(t *testing.T) {
	st := &migStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", map[string]any{"spec": validSpecBody()}, "mig_ref", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.updateModSpec.called {
		t.Error("store.UpdateMigSpec was not called")
	}

	resp := decodeBody[struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}](t, rr)
	if resp.ID == "" {
		t.Error("response ID is empty")
	}
	if resp.CreatedAt == "" {
		t.Error("response created_at is empty")
	}
}

// TestMods_SetSpec_WithName verifies POST /v1/migs/{mig_ref}/specs accepts optional name.
func TestMods_SetSpec_WithName(t *testing.T) {
	st := &migStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	reqBody := map[string]any{
		"name": "my-named-spec",
		"spec": validSpecBody(),
	}

	rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", reqBody, "mig_ref", "mod123")
	assertStatus(t, rr, http.StatusCreated)

	if st.createSpecParams.Name != "my-named-spec" {
		t.Errorf("spec name = %q, want %q", st.createSpecParams.Name, "my-named-spec")
	}
}

// TestMods_SetSpec_RepeatedCalls verifies that repeated set spec calls create new spec rows and update migs.spec_id.
func TestMods_SetSpec_RepeatedCalls(t *testing.T) {
	st := &migStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	// First call.
	rr1 := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", map[string]any{"spec": validSpecBody()}, "mig_ref", "mod123")
	assertStatus(t, rr1, http.StatusCreated)

	firstSpecID := decodeBody[struct{ ID string }](t, rr1).ID

	// Reset mock tracking for second call.
	st.createSpecCalled = false
	st.updateModSpec.called = false

	// Second call with different spec.
	spec2 := validSpecBody()
	spec2["env"] = map[string]any{"FOO": "bar"}
	rr2 := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", map[string]any{"spec": spec2}, "mig_ref", "mod123")
	assertStatus(t, rr2, http.StatusCreated)

	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called on second invocation")
	}
	if !st.updateModSpec.called {
		t.Error("store.UpdateMigSpec was not called on second invocation")
	}

	secondSpecID := decodeBody[struct{ ID string }](t, rr2).ID
	if firstSpecID == secondSpecID {
		t.Error("repeated calls should produce different spec IDs")
	}
}

// TestMods_SetSpec_ErrorPaths merges individual error tests into a table-driven test.
func TestMods_SetSpec_ErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		store      *migStore
		body       any
		wantStatus int
	}{
		{
			name:       "MissingSpec",
			store:      &migStore{},
			body:       map[string]any{"name": "no-spec"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidSpec",
			store:      &migStore{},
			body:       map[string]any{"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidJSON",
			store:      &migStore{},
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ModNotFound",
			store:      &migStore{getModErr: pgx.ErrNoRows},
			body:       map[string]any{"spec": validSpecBody()},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ArchivedMod",
			store: &migStore{
				getModResult: store.Mig{
					ID:         "mod123",
					Name:       "test-mig",
					ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			},
			body:       map[string]any{"spec": validSpecBody()},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := setMigSpecHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", tt.body, "mig_ref", "mod123")
			assertStatus(t, rr, tt.wantStatus)
			if tt.store.createSpecCalled {
				t.Error("store.CreateSpec should not be called")
			}
		})
	}
}

// TestMods_SetSpec_StoreErrors merges CreateSpec and UpdateMigSpec store error tests.
func TestMods_SetSpec_StoreErrors(t *testing.T) {
	tests := []struct {
		name  string
		store *migStore
	}{
		{
			name: "CreateSpecError",
			store: &migStore{
				getModResult: store.Mig{
					ID:         "mod123",
					Name:       "test-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				},
				createSpecErr: errors.New("database connection failed"),
			},
		},
		{
			name: "UpdateMigSpecError",
			store: func() *migStore {
				st := &migStore{
					getModResult: store.Mig{
						ID:         "mod123",
						Name:       "test-mig",
						ArchivedAt: pgtype.Timestamptz{Valid: false},
					},
				}
				st.updateModSpec.err = errors.New("database connection failed")
				return st
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := setMigSpecHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mod123/specs", map[string]any{"spec": validSpecBody()}, "mig_ref", "mod123")
			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}
