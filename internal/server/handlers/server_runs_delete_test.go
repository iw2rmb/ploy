package handlers

import (
	"net/http"
	"strings"
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

	st := &mockStore{}
	st.getRun.val = store.Run{
		ID: runID,
		}

	handler := deleteRunHandler(st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/runs/"+runID.String(), nil, "id", runID.String())

	assertStatus(t, rr, http.StatusNoContent)

	// Verify DeleteRun was called with correct run ID.
	if !st.deleteRun.called {
		t.Fatal("expected DeleteRun to be called")
	}
	if st.deleteRun.params != runID.String() {
		t.Fatalf("DeleteRun called with wrong run id: %v", st.deleteRun.params)
	}
}

// TestDeleteRun_Errors merges NotFound, EmptyID, MissingID error tests.
func TestDeleteRun_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pathID     string
		store      *mockStore
		wantStatus int
	}{
		{
			name:       "NotFound",
			pathID:     domaintypes.NewRunID().String(),
			store:      func() *mockStore { st := &mockStore{}; st.getRun.err = pgx.ErrNoRows; return st }(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "EmptyID",
			pathID:     "   ",
			store:      &mockStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingID",
			pathID:     "",
			store:      &mockStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := deleteRunHandler(tt.store)
			// Use a safe URL path; the path value is set separately by doRequest.
			urlID := tt.pathID
			if strings.TrimSpace(urlID) == "" {
				urlID = "_"
			}
			rr := doRequest(t, handler, http.MethodDelete, "/v1/runs/"+urlID, nil, "id", tt.pathID)
			assertStatus(t, rr, tt.wantStatus)
		})
	}
}
