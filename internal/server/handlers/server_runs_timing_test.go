package handlers

import (
	"net/http"
	"net/http/httptest"
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

	st := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID:      runID,
			QueueMs: 1500,
			RunMs:   3000,
		},
	}

	handler := getRunTimingHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/timing", nil, "id", runID.String())

	assertStatus(t, rr, http.StatusOK)

	// Verify GetRunTiming was called with correct run ID.
	if !st.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
	if st.getRunTimingParams != runID.String() {
		t.Fatalf("GetRunTiming called with wrong run id: %v", st.getRunTimingParams)
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

// TestGetRunTiming_NotFound verifies 404 when run timing is missing.
func TestGetRunTiming_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	st := &mockStore{
		getRunTimingErr: pgx.ErrNoRows,
	}

	handler := getRunTimingHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/timing", nil, "id", runID.String())

	assertStatus(t, rr, http.StatusNotFound)
}

// TestGetRunTiming_EmptyID verifies 400 for empty or whitespace ID.
// Run IDs are KSUID strings; empty/whitespace IDs are rejected.
func TestGetRunTiming_EmptyID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunTimingHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//timing", nil)
	req.SetPathValue("id", "   ") // Whitespace ID
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestGetRunTiming_MissingID verifies 400 is returned when id is missing.
func TestGetRunTiming_MissingID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunTimingHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs//timing", nil, "id", "")

	assertStatus(t, rr, http.StatusBadRequest)
}
