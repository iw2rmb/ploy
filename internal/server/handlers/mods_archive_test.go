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
// PATCH /v1/mods/{mod_ref}/archive — Archive Mod
// =============================================================================

// TestMods_Archive_Success verifies PATCH /v1/mods/{mod_ref}/archive archives a mod.
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
	req.SetPathValue("mod_ref", "mod123")
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
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_Archive_NotFound verifies PATCH /v1/mods/{mod_ref}/archive returns 404.
func TestMods_Archive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/nonexistent/archive", nil)
	req.SetPathValue("mod_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestMods_Archive_RefusesWithRunningJobs verifies PATCH /v1/mods/{mod_ref}/archive
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
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_Archive_RefusesWithQueuedJobs verifies PATCH /v1/mods/{mod_ref}/archive
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
	req.SetPathValue("mod_ref", "mod123")
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
	req.SetPathValue("mod_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !st.archiveModCalled {
		t.Error("store.ArchiveMod should be called when only completed jobs exist")
	}
}

func TestMods_Archive_ByName(t *testing.T) {
	st := &mockStore{
		getModErr:          pgx.ErrNoRows,
		getModByNameResult: store.Mod{ID: "mod123", Name: "my-mod", ArchivedAt: pgtype.Timestamptz{Valid: false}},
		listRunsResult:     []store.Run{},
	}
	handler := archiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/my-mod/archive", nil)
	req.SetPathValue("mod_ref", "my-mod")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !st.getModByNameCalled {
		t.Error("store.GetModByName was not called")
	}
	if st.archiveModParam != "mod123" {
		t.Errorf("ArchiveMod param = %q, want %q", st.archiveModParam, "mod123")
	}
}

// TestMods_Archive_StoreError verifies PATCH /v1/mods/{mod_ref}/archive returns 500 on store error.
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
	req.SetPathValue("mod_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// PATCH /v1/mods/{mod_ref}/unarchive — Unarchive Mod
// =============================================================================

// TestMods_Unarchive_Success verifies PATCH /v1/mods/{mod_ref}/unarchive unarchives a mod.
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
	req.SetPathValue("mod_ref", "mod123")
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
	req.SetPathValue("mod_ref", "mod123")
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

// TestMods_Unarchive_NotFound verifies PATCH /v1/mods/{mod_ref}/unarchive returns 404.
func TestMods_Unarchive_NotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := unarchiveModHandler(st)

	req := httptest.NewRequest(http.MethodPatch, "/v1/mods/nonexistent/unarchive", nil)
	req.SetPathValue("mod_ref", "nonexistent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestMods_Unarchive_StoreError verifies PATCH /v1/mods/{mod_ref}/unarchive returns 500 on store error.
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
	req.SetPathValue("mod_ref", "mod123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
