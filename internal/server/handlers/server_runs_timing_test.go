package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Run Timing Tests =====
// getRunTimingHandler retrieves timing data (queue_ms, run_ms) for a run.

// TestGetRunTiming_Success verifies timing data is returned for a valid run.
func TestGetRunTiming_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &runStore{}
	st.getRunTiming.val = store.RunsTiming{
		ID:      runID,
		QueueMs: 1500,
		RunMs:   3000,
		}

	handler := getRunTimingHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/timing", nil, "id", runID.String())

	assertStatus(t, rr, http.StatusOK)

	// Verify GetRunTiming was called with correct run ID.
	if !st.getRunTiming.called {
		t.Fatal("expected GetRunTiming to be called")
	}
	if st.getRunTiming.params != runID.String() {
		t.Fatalf("GetRunTiming called with wrong run id: %v", st.getRunTiming.params)
	}

	// Parse and verify response body.
	resp := decodeBody[map[string]any](t, rr)

	if resp["id"] != runID.String() {
		t.Errorf("expected id %s, got %v", runID.String(), resp["id"])
	}
	if resp["queue_ms"] != float64(1500) {
		t.Errorf("expected queue_ms 1500, got %v", resp["queue_ms"])
	}
	if resp["run_ms"] != float64(3000) {
		t.Errorf("expected run_ms 3000, got %v", resp["run_ms"])
	}
}

// TestGetRunTiming_Errors merges NotFound, EmptyID, MissingID error tests.
func TestGetRunTiming_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pathID     string
		store      *runStore
		wantStatus int
	}{
		{
			name:       "NotFound",
			pathID:     domaintypes.NewRunID().String(),
			store:      func() *runStore { st := &runStore{}; st.getRunTiming.err = pgx.ErrNoRows; return st }(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "EmptyID",
			pathID:     "   ",
			store:      &runStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingID",
			pathID:     "",
			store:      &runStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := getRunTimingHandler(tt.store)
			// Use a safe URL path; the path value is set separately by doRequest.
			urlID := tt.pathID
			if strings.TrimSpace(urlID) == "" {
				urlID = "_"
			}
			rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+urlID+"/timing", nil, "id", tt.pathID)
			assertStatus(t, rr, tt.wantStatus)
		})
	}
}
