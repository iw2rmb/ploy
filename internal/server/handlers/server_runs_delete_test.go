package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Run Deletion Tests =====
// deleteRunHandler deletes a run by id.

// TestDeleteRun_Success verifies a run is deleted successfully.
func TestDeleteRun_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &mockStore{
		getRunResult: store.Run{
			ID: runID,
		},
	}

	handler := deleteRunHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/runs/"+runID.String(), nil, "id", runID.String())

	assertStatus(t, rr, http.StatusNoContent)

	// Verify DeleteRun was called with correct run ID.
	if !st.deleteRunCalled {
		t.Fatal("expected DeleteRun to be called")
	}
	if st.deleteRunParams != runID.String() {
		t.Fatalf("DeleteRun called with wrong run id: %v", st.deleteRunParams)
	}
}

// TestDeleteRun_NotFound verifies 404 is returned when the run does not exist.
func TestDeleteRun_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := deleteRunHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/runs/"+runID.String(), nil, "id", runID.String())

	assertStatus(t, rr, http.StatusNotFound)

	// Verify DeleteRun was not called since GetRun failed.
	if st.deleteRunCalled {
		t.Fatal("did not expect DeleteRun to be called")
	}
}

// TestDeleteRun_EmptyID verifies 400 is returned for an empty or whitespace run ID.
// Run IDs are KSUID strings; empty/whitespace IDs are rejected.
func TestDeleteRun_EmptyID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/", nil)
	req.SetPathValue("id", "   ") // Whitespace ID
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestDeleteRun_MissingID verifies 400 is returned when id path parameter is missing.
func TestDeleteRun_MissingID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := deleteRunHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/runs/", nil, "id", "")

	assertStatus(t, rr, http.StatusBadRequest)
}
