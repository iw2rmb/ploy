package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Run Timing Tests =====
// getRunTimingHandler retrieves timing data (queue_ms, run_ms) for a run.

// TestGetRunTiming_Success verifies timing data is returned for a valid run.
func TestGetRunTiming_Success(t *testing.T) {
	t.Parallel()

	runID := uuid.New()

	st := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID:      pgtype.UUID{Bytes: runID, Valid: true},
			QueueMs: 1500,
			RunMs:   3000,
		},
	}

	handler := getRunTimingHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/timing", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify GetRunTiming was called with correct run ID.
	if !st.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
	if st.getRunTimingParams.Bytes != runID {
		t.Fatalf("GetRunTiming called with wrong run id: %v", st.getRunTimingParams)
	}

	// Parse and verify response body.
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

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

	runID := uuid.New()

	st := &mockStore{
		getRunTimingErr: pgx.ErrNoRows,
	}

	handler := getRunTimingHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/timing", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetRunTiming_InvalidID verifies 400 for invalid UUID.
func TestGetRunTiming_InvalidID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunTimingHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/invalid-uuid/timing", nil)
	req.SetPathValue("id", "invalid-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetRunTiming_MissingID verifies 400 is returned when id is missing.
func TestGetRunTiming_MissingID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunTimingHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//timing", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
