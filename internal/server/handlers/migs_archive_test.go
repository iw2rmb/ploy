package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
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

// TestMods_Archive_Success verifies PATCH /v1/migs/{mig_ref}/archive archives a mig.
// Tests mig archiving to prevent execution.
func TestMods_Archive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
		// No runs/jobs for this mig.
		listRunsResult: []store.Run{},
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/archive", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusOK)

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.archiveMigCalled {
		t.Error("store.ArchiveMig was not called")
	}
	if st.archiveMigParam != "mod123" {
		t.Errorf("ArchiveMig param = %q, want %q", st.archiveMigParam, "mod123")
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
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Already archived.
		},
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/archive", nil, "mig_ref", "mod123")

	// Should return OK (idempotent) without calling ArchiveMig again.
	assertStatus(t, rr, http.StatusOK)
	if st.archiveMigCalled {
		t.Error("store.ArchiveMig should not be called for already-archived mig")
	}
}

// TestMods_Archive_NotFound verifies PATCH /v1/migs/{mig_ref}/archive returns 404.
func TestMods_Archive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/nonexistent/archive", nil, "mig_ref", "nonexistent")

	assertStatus(t, rr, http.StatusNotFound)
}

// TestMods_Archive_RefusesWithActiveJobs verifies PATCH /v1/migs/{mig_ref}/archive
// returns 409 when active jobs exist.
func TestMods_Archive_RefusesWithActiveJobs(t *testing.T) {
	tests := []struct {
		name      string
		jobStatus domaintypes.JobStatus
	}{
		{name: "running", jobStatus: domaintypes.JobStatusRunning},
		{name: "queued", jobStatus: domaintypes.JobStatusQueued},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			st := &mockStore{
				getModResult: store.Mig{
					ID:         "mod123",
					Name:       "test-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: false},
				},
				listRunsResult: []store.Run{
					{ID: "run1", MigID: "mod123"},
				},
				listJobsByRunResult: []store.Job{
					{ID: "job1", RunID: "run1", Status: tt.jobStatus},
				},
			}
			handler := archiveMigHandler(st)

			rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/archive", nil, "mig_ref", "mod123")

			assertStatus(t, rr, http.StatusConflict)
			if st.archiveMigCalled {
				t.Fatal("store.ArchiveMig should not be called when active jobs exist")
			}
		})
	}
}

// TestMods_Archive_AllowsWithCompletedJobs verifies archive succeeds when
// only completed jobs exist.
func TestMods_Archive_AllowsWithCompletedJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{
			{ID: "run1", MigID: "mod123"},
		},
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: domaintypes.JobStatusSuccess},
			{ID: "job2", RunID: "run1", Status: domaintypes.JobStatusFail},
		},
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/archive", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusOK)
	if !st.archiveMigCalled {
		t.Error("store.ArchiveMig should be called when only completed jobs exist")
	}
}

func TestMods_Archive_ByName(t *testing.T) {
	st := &mockStore{
		getModErr:          pgx.ErrNoRows,
		getModByNameResult: store.Mig{ID: "mod123", Name: "my-mig", ArchivedAt: pgtype.Timestamptz{Valid: false}},
		listRunsResult:     []store.Run{},
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/my-mig/archive", nil, "mig_ref", "my-mig")

	assertStatus(t, rr, http.StatusOK)
	if !st.getModByNameCalled {
		t.Error("store.GetMigByName was not called")
	}
	if st.archiveMigParam != "mod123" {
		t.Errorf("ArchiveMig param = %q, want %q", st.archiveMigParam, "mod123")
	}
}

// TestMods_Archive_StoreError verifies PATCH /v1/migs/{mig_ref}/archive returns 500 on store error.
func TestMods_Archive_StoreError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{},
		archiveMigErr:  errors.New("database connection failed"),
	}
	handler := archiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/archive", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusInternalServerError)
}

// =============================================================================
// PATCH /v1/migs/{mig_ref}/unarchive — Unarchive Mig
// =============================================================================

// TestMods_Unarchive_Success verifies PATCH /v1/migs/{mig_ref}/unarchive unarchives a mig.
// Tests mig unarchiving to allow execution again.
func TestMods_Unarchive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := unarchiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/unarchive", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusOK)

	// Verify store methods called.
	if !st.unarchiveMigCalled {
		t.Error("store.UnarchiveMig was not called")
	}
	if st.unarchiveMigParam != "mod123" {
		t.Errorf("UnarchiveMig param = %q, want %q", st.unarchiveMigParam, "mod123")
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
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
	}
	handler := unarchiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/unarchive", nil, "mig_ref", "mod123")

	// Should return OK (idempotent) without calling UnarchiveMig.
	assertStatus(t, rr, http.StatusOK)
	if st.unarchiveMigCalled {
		t.Error("store.UnarchiveMig should not be called for already-unarchived mig")
	}
}

// TestMods_Unarchive_NotFound verifies PATCH /v1/migs/{mig_ref}/unarchive returns 404.
func TestMods_Unarchive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := unarchiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/nonexistent/unarchive", nil, "mig_ref", "nonexistent")

	assertStatus(t, rr, http.StatusNotFound)
}

// TestMods_Unarchive_StoreError verifies PATCH /v1/migs/{mig_ref}/unarchive returns 500 on store error.
func TestMods_Unarchive_StoreError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		unarchiveMigErr: errors.New("database connection failed"),
	}
	handler := unarchiveMigHandler(st)

	rr := doRequest(t, handler, http.MethodPatch, "/v1/migs/mod123/unarchive", nil, "mig_ref", "mod123")

	assertStatus(t, rr, http.StatusInternalServerError)
}
