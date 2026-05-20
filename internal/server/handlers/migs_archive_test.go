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
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = activeMig
				return st
			}(),
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMig", st.getMig.called)
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
			name: "already archived (idempotent)",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = archivedMig
				return st
			}(),
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name: "not found",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.err = pgx.ErrNoRows
				return st
			}(),
			migRef:     "nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "refuses with active jobs",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = activeMig
				st.listRuns.val = []store.Run{{ID: "run1", MigID: "mig123"}}
				st.listJobsByRun.val = []store.Job{
					{ID: "job1", RunID: "run1", Status: domaintypes.JobStatusRunning},
				}
				return st
			}(),
			migRef:     "mig123",
			wantStatus: http.StatusConflict,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name: "allows with completed jobs",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = activeMig
				st.listRuns.val = []store.Run{{ID: "run1", MigID: "mig123"}}
				st.listJobsByRun.val = []store.Job{
					{ID: "job1", RunID: "run1", Status: domaintypes.JobStatusSuccess},
					{ID: "job2", RunID: "run1", Status: domaintypes.JobStatusFail},
				}
				return st
			}(),
			migRef:     "mig123",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ArchiveMig", st.archiveMig.called)
			},
		},
		{
			name: "by name",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.err = pgx.ErrNoRows
				st.getMigByName.val = store.Mig{ID: "mig123", Name: "my-mig", ArchivedAt: pgtype.Timestamptz{Valid: false}}
				return st
			}(),
			migRef:     "my-mig",
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "GetMigByName", st.getMigByName.called)
				if st.archiveMig.params != "mig123" {
					t.Errorf("ArchiveMig param = %q, want %q", st.archiveMig.params, "mig123")
				}
			},
		},
		{
			name: "store error",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = activeMig
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
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = store.Mig{
					ID: "mig123", Name: "test-mig",
					ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}
				return st
			}(),
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
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = store.Mig{
					ID: "mig123", Name: "test-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				}
				return st
			}(),
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *migStore, _ *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "UnarchiveMig", st.unarchiveMig.called)
			},
		},
		{
			name: "not found",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.err = pgx.ErrNoRows
				return st
			}(),
			wantStatus: http.StatusNotFound,
		},
		{
			name: "store error",
			store: func() *migStore {
				st := &migStore{}
				st.getMig.val = store.Mig{
					ID: "mig123", Name: "test-mig",
					ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
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
