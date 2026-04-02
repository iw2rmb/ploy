package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// PATCH /v1/migs/{mig_ref}/archive — Archive Mig
// =============================================================================

func TestMigs_Archive(t *testing.T) {
	activeMig := store.Mig{
		ID: "mig123", Name: "test-mig",
		ArchivedAt: pgtype.Timestamptz{Valid: false},
	}
	archivedMig := store.Mig{
		ID: "mig123", Name: "test-mig",
		ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	tests := []struct {
		name       string
		store      *migStore
		migRef     string
		wantStatus int
		verify     func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			store: &migStore{
				getMigResult:   activeMig,
				listRunsResult: []store.Run{},
			},
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMig", st.getMigCalled)
				assertCalled(t, "ArchiveMig", st.archiveMig.called)
				if st.archiveMig.params != "mig123" {
					t.Errorf("ArchiveMig param = %q, want %q", st.archiveMig.params, "mig123")
				}
				resp := decodeBody[struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Archived bool   `json:"archived"`
				}](t, rr)
				if !resp.Archived {
					t.Error("response Archived = false, want true")
				}
			},
		},
		{
			name:       "already archived (idempotent)",
			store:      &migStore{getMigResult: archivedMig},
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name:       "not found",
			store:      &migStore{getMigErr: pgx.ErrNoRows},
			migRef:     "nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "refuses with active jobs",
			store: &migStore{
				getMigResult:   activeMig,
				listRunsResult: []store.Run{{ID: "run1", MigID: "mig123"}},
				listJobsByRunResult: []store.Job{
					{ID: "job1", RunID: "run1", Status: domaintypes.JobStatusRunning},
				},
			},
			migRef:     "mig123",
			wantStatus: http.StatusConflict,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name: "allows with completed jobs",
			store: &migStore{
				getMigResult:   activeMig,
				listRunsResult: []store.Run{{ID: "run1", MigID: "mig123"}},
				listJobsByRunResult: []store.Job{
					{ID: "job1", RunID: "run1", Status: domaintypes.JobStatusSuccess},
					{ID: "job2", RunID: "run1", Status: domaintypes.JobStatusFail},
				},
			},
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name: "by name",
			store: &migStore{
				getMigErr:          pgx.ErrNoRows,
				getMigByNameResult: store.Mig{ID: "mig123", Name: "my-mig", ArchivedAt: pgtype.Timestamptz{Valid: false}},
				listRunsResult:     []store.Run{},
			},
			migRef:     "my-mig",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMigByName", st.getMigByNameCalled)
				if st.archiveMig.params != "mig123" {
					t.Errorf("ArchiveMig param = %q, want %q", st.archiveMig.params, "mig123")
				}
			},
		},
		{
			name: "store error",
			store: func() *migStore {
				st := &migStore{
					getMigResult:   activeMig,
					listRunsResult: []store.Run{},
				}
				st.archiveMig.err = errors.New("database connection failed")
				return st
			}(),
			migRef:     "mig123",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := archiveMigHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/"+tt.migRef+"/archive", nil, "mig_ref", tt.migRef)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

// =============================================================================
// PATCH /v1/migs/{mig_ref}/unarchive — Unarchive Mig
// =============================================================================

func TestMigs_Unarchive(t *testing.T) {
	tests := []struct {
		name       string
		store      *migStore
		wantStatus int
		verify     func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			store: &migStore{
				getMigResult: store.Mig{
					ID: "mig123", Name: "test-mig",
					ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "UnarchiveMig", st.unarchiveMig.called)
				if st.unarchiveMig.params != "mig123" {
					t.Errorf("UnarchiveMig param = %q, want %q", st.unarchiveMig.params, "mig123")
				}
				resp := decodeBody[struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Archived bool   `json:"archived"`
				}](t, rr)
				if resp.Archived {
					t.Error("response Archived = true, want false")
				}
			},
		},
		{
			name: "already unarchived (idempotent)",
			store: &migStore{
				getMigResult: store.Mig{
					ID: "mig123", Name: "test-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				},
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "UnarchiveMig", st.unarchiveMig.called)
			},
		},
		{
			name:       "not found",
			store:      &migStore{getMigErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "store error",
			store: func() *migStore {
				st := &migStore{
					getMigResult: store.Mig{
						ID: "mig123", Name: "test-mig",
						ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}
				st.unarchiveMig.err = errors.New("database connection failed")
				return st
			}(),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := unarchiveMigHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mig123/unarchive", nil, "mig_ref", "mig123")
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}
