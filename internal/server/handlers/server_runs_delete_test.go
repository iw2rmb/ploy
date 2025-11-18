package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Run Deletion Tests =====
// deleteRunHandler deletes a run by id.

// TestDeleteRun_Success verifies a run is deleted successfully.
func TestDeleteRun_Success(t *testing.T) {
	t.Parallel()

	runID := uuid.New()

	st := &mockStore{
		getRunResult: store.Run{
			ID: pgtype.UUID{Bytes: runID, Valid: true},
		},
	}

	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify DeleteRun was called with correct run ID.
	if !st.deleteRunCalled {
		t.Fatal("expected DeleteRun to be called")
	}
	if st.deleteRunParams.Bytes != runID {
		t.Fatalf("DeleteRun called with wrong run id: %v", st.deleteRunParams)
	}
}

// TestDeleteRun_NotFound verifies 404 is returned when the run does not exist.
func TestDeleteRun_NotFound(t *testing.T) {
	t.Parallel()

	runID := uuid.New()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify DeleteRun was not called since GetRun failed.
	if st.deleteRunCalled {
		t.Fatal("did not expect DeleteRun to be called")
	}
}

// TestDeleteRun_InvalidID verifies 400 is returned for an invalid run ID.
func TestDeleteRun_InvalidID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/invalid-uuid", nil)
	req.SetPathValue("id", "invalid-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteRun_MissingID verifies 400 is returned when id path parameter is missing.
func TestDeleteRun_MissingID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
