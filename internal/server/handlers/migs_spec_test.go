package handlers

import (
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

func TestMigs_GetLatestSpec(t *testing.T) {
	specRow := store.Spec{ID: "spec123", Spec: []byte(`{"version":"0.2.0","steps":[{"image":"alpine"}]}`)}
	migSpecID := specRow.ID

	tests := []struct {
		name       string
		store      *migStore
		wantStatus int
		verify     func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			store: func() *migStore {
				st := &migStore{
					getMigResult: store.Mig{
						ID: "mig123", Name: "test-mig",
						SpecID:     &migSpecID,
						ArchivedAt: pgtype.Timestamptz{Valid: false},
					},
				}
				st.getSpec.val = specRow
				return st
			}(),
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				if got := rr.Header().Get("Content-Type"); got != "application/json" {
					t.Fatalf("content-type = %q, want application/json", got)
				}
				if rr.Body.String() != string(specRow.Spec) {
					t.Fatalf("body = %q, want %q", rr.Body.String(), string(specRow.Spec))
				}
				assertCalled(t, "GetMig", st.getMigCalled)
				assertCalled(t, "GetSpec", st.getSpec.called)
			},
		},
		{
			name: "mig without spec",
			store: &migStore{
				getMigResult: store.Mig{
					ID: "mig123", Name: "test-mig",
					SpecID:     nil,
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				},
			},
			wantStatus: http.StatusNotFound,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "GetSpec", st.getSpec.called)
			},
		},
		{
			name:       "mig not found",
			store:      &migStore{getMigErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "GetSpec", st.getSpec.called)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := getMigLatestSpecHandler(tt.store)
			rr := doRequest(t, handler, http.MethodGet, "/v1/migs/mig123/specs/latest", nil, "mig_ref", "mig123")
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

// =============================================================================
// POST /v1/migs/{mig_ref}/specs — Set Mig Spec
// =============================================================================

func TestMigs_SetSpec(t *testing.T) {
	activeMig := store.Mig{
		ID: "mig123", Name: "test-mig",
		ArchivedAt: pgtype.Timestamptz{Valid: false},
	}

	tests := []struct {
		name       string
		store      *migStore // nil = default active mig store
		body       any
		wantStatus int
		verify     func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		// Success paths
		{
			name:       "success",
			body:       map[string]any{"spec": validSpecBody()},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMig", st.getMigCalled)
				assertCalled(t, "CreateSpec", st.createSpecCalled)
				assertCalled(t, "UpdateMigSpec", st.updateMigSpec.called)
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
			},
		},
		{
			name:       "with name",
			body:       map[string]any{"name": "my-named-spec", "spec": validSpecBody()},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				if st.createSpecParams.Name != "my-named-spec" {
					t.Errorf("spec name = %q, want %q", st.createSpecParams.Name, "my-named-spec")
				}
			},
		},
		// Error paths
		{name: "missing spec", store: &migStore{}, body: map[string]any{"name": "no-spec"}, wantStatus: http.StatusBadRequest},
		{name: "invalid spec", store: &migStore{}, body: map[string]any{"spec": map[string]any{"mig": map[string]any{"command": "echo hello"}}}, wantStatus: http.StatusBadRequest},
		{name: "invalid JSON", store: &migStore{}, body: "not json", wantStatus: http.StatusBadRequest},
		{name: "mig not found", store: &migStore{getMigErr: pgx.ErrNoRows}, body: map[string]any{"spec": validSpecBody()}, wantStatus: http.StatusNotFound},
		{
			name: "archived mig",
			store: &migStore{getMigResult: store.Mig{
				ID: "mig123", Name: "test-mig",
				ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}},
			body: map[string]any{"spec": validSpecBody()}, wantStatus: http.StatusConflict,
		},
		{
			name: "CreateSpec store error",
			store: &migStore{
				getMigResult:  activeMig,
				createSpecErr: errors.New("database connection failed"),
			},
			body: map[string]any{"spec": validSpecBody()}, wantStatus: http.StatusInternalServerError,
		},
		{
			name: "UpdateMigSpec store error",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.updateMigSpec.err = errors.New("database connection failed")
				return st
			}(),
			body: map[string]any{"spec": validSpecBody()}, wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.store
			if st == nil {
				st = &migStore{getMigResult: activeMig}
			}
			handler := setMigSpecHandler(st)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/mig123/specs", tt.body, "mig_ref", "mig123")
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}

func TestMigs_SetSpec_RepeatedCalls(t *testing.T) {
	st := &migStore{
		getMigResult: store.Mig{
			ID: "mig123", Name: "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := setMigSpecHandler(st)

	rr1 := doRequest(t, handler, http.MethodPost, "/v1/migs/mig123/specs", map[string]any{"spec": validSpecBody()}, "mig_ref", "mig123")
	assertStatus(t, rr1, http.StatusCreated)
	firstSpecID := decodeBody[struct{ ID string }](t, rr1).ID

	st.createSpecCalled = false
	st.updateMigSpec.called = false

	spec2 := validSpecBody()
	spec2["envs"] = map[string]any{"FOO": "bar"}
	rr2 := doRequest(t, handler, http.MethodPost, "/v1/migs/mig123/specs", map[string]any{"spec": spec2}, "mig_ref", "mig123")
	assertStatus(t, rr2, http.StatusCreated)

	assertCalled(t, "CreateSpec", st.createSpecCalled)
	assertCalled(t, "UpdateMigSpec", st.updateMigSpec.called)

	secondSpecID := decodeBody[struct{ ID string }](t, rr2).ID
	if firstSpecID == secondSpecID {
		t.Error("repeated calls should produce different spec IDs")
	}
}
