package handlers

import (
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
// PATCH /v1/migs/{mig_ref}/archive — Archive Mig
// =============================================================================

// TestMods_Archive_Success verifies PATCH /v1/migs/{mig_ref}/archive archives a mod.
// Tests mod archiving to prevent execution.
func TestMods_Archive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
		// No runs/jobs for this mod.
		listRunsResult: []store.Run{},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

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
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Already archived.
		},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return OK (idempotent) without calling ArchiveMig again.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if st.archiveMigCalled {
		t.Error("store.ArchiveMig should not be called for already-archived mod")
	}
}

// TestMods_Archive_NotFound verifies PATCH /v1/migs/{mig_ref}/archive returns 404.
func TestMods_Archive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/nonexistent/archive", nil)
	req.SetPathValue("mig_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestMods_Archive_RefusesWithRunningJobs verifies PATCH /v1/migs/{mig_ref}/archive
// returns 409 when running jobs exist.
// Archive refuses if running jobs exist.
func TestMods_Archive_RefusesWithRunningJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		// A run exists for this mod.
		listRunsResult: []store.Run{
			{ID: "run1", MigID: "mod123"},
		},
		// That run has running jobs.
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusRunning},
		},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}

	// ArchiveMig should not be called.
	if st.archiveMigCalled {
		t.Error("store.ArchiveMig should not be called when running jobs exist")
	}
}

// TestMods_Archive_RefusesWithQueuedJobs verifies PATCH /v1/migs/{mig_ref}/archive
// returns 409 when queued jobs exist (also considered "running").
func TestMods_Archive_RefusesWithQueuedJobs(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{
			{ID: "run1", MigID: "mod123"},
		},
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusQueued},
		},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
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
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{
			{ID: "run1", MigID: "mod123"},
		},
		listJobsByRunResult: []store.Job{
			{ID: "job1", RunID: "run1", Status: store.JobStatusSuccess},
			{ID: "job2", RunID: "run1", Status: store.JobStatusFail},
		},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !st.archiveMigCalled {
		t.Error("store.ArchiveMig should be called when only completed jobs exist")
	}
}

func TestMods_Archive_ByName(t *testing.T) {
	st := &mockStore{
		getModErr:          pgx.ErrNoRows,
		getModByNameResult: store.Mig{ID: "mod123", Name: "my-mod", ArchivedAt: pgtype.Timestamptz{Valid: false}},
		listRunsResult:     []store.Run{},
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/my-mod/archive", nil)
	req.SetPathValue("mig_ref", "my-mod")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
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
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		listRunsResult: []store.Run{},
		archiveMigErr:  errors.New("database connection failed"),
	}
	handler := archiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/archive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// PATCH /v1/migs/{mig_ref}/unarchive — Unarchive Mig
// =============================================================================

// TestMods_Unarchive_Success verifies PATCH /v1/migs/{mig_ref}/unarchive unarchives a mod.
// Tests mod unarchiving to allow execution again.
func TestMods_Unarchive_Success(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := unarchiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/unarchive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

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
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Valid: false}, // Not archived.
		},
	}
	handler := unarchiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/unarchive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return OK (idempotent) without calling UnarchiveMig.
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if st.unarchiveMigCalled {
		t.Error("store.UnarchiveMig should not be called for already-unarchived mod")
	}
}

// TestMods_Unarchive_NotFound verifies PATCH /v1/migs/{mig_ref}/unarchive returns 404.
func TestMods_Unarchive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := unarchiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/nonexistent/unarchive", nil)
	req.SetPathValue("mig_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestMods_Unarchive_StoreError verifies PATCH /v1/migs/{mig_ref}/unarchive returns 500 on store error.
func TestMods_Unarchive_StoreError(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mod",
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		unarchiveMigErr: errors.New("database connection failed"),
	}
	handler := unarchiveMigHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/migs/mod123/unarchive", nil)
	req.SetPathValue("mig_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
