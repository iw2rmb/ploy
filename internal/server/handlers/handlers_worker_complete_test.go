package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestCompleteRun_Success verifies a node can complete a running run it owns
// and that UpdateRunCompletion is invoked with the expected parameters.
func TestCompleteRun_Success(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	payload := map[string]any{
		"run_id": runID.String(),
		"status": "succeeded",
		"stats":  map[string]any{"exit_code": 0, "duration_ms": 1234},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunCompletionCalled {
		t.Fatal("expected UpdateRunCompletion to be called")
	}
	if st.updateRunCompletionParams.ID.Bytes != runID {
		t.Fatalf("UpdateRunCompletion called with wrong run id: %v", st.updateRunCompletionParams.ID)
	}
	if st.updateRunCompletionParams.Status != store.RunStatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", st.updateRunCompletionParams.Status)
	}
	// Verify stats is a JSON object containing our fields.
	var stats map[string]any
	if err := json.Unmarshal(st.updateRunCompletionParams.Stats, &stats); err != nil {
		t.Fatalf("stats not valid JSON object: %v", err)
	}
	if stats["exit_code"] != float64(0) { // json numbers decode to float64
		t.Fatalf("expected stats.exit_code=0, got %v", stats["exit_code"])
	}
}

// TestCompleteRun_WrongNode returns 403 when the run is owned by another node.
func TestCompleteRun_WrongNode(t *testing.T) {
	nodeID := uuid.New()
	other := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: other, Valid: true},
			Status: store.RunStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "failed", "reason": "boom"})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called")
	}
}

// TestCompleteRun_NotRunning returns 409 when the run is not in running state.
func TestCompleteRun_NotRunning(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusAssigned,
		},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "failed"})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called")
	}
}

// TestCompleteRun_InvalidStatus returns 400 when non-terminal status provided.
func TestCompleteRun_InvalidStatus(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "running"})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called")
	}
}

// TestCompleteRun_StatsMustBeObject returns 400 when stats is not a JSON object.
func TestCompleteRun_StatsMustBeObject(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	// stats provided as a string, which is valid JSON but not an object.
	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "failed", "stats": "oops"})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called")
	}
}

// TestCompleteRun_NotFound checks 404 paths for missing node/run.
func TestCompleteRun_NotFound(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	// Node not found
	st1 := &mockStore{getNodeErr: pgx.ErrNoRows}
	handler1 := completeRunHandler(st1, nil)
	b1, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "failed"})
	req1 := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(b1))
	req1.SetPathValue("id", nodeID.String())
	rr1 := httptest.NewRecorder()
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing node, got %d", rr1.Code)
	}

	// Run not found
	st2 := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunErr:     pgx.ErrNoRows,
	}
	handler2 := completeRunHandler(st2, nil)
	b2, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "failed"})
	req2 := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(b2))
	req2.SetPathValue("id", nodeID.String())
	rr2 := httptest.NewRecorder()
	handler2.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing run, got %d", rr2.Code)
	}
}

// TestCompleteRun_PublishesEvents verifies that completing a run publishes both
// a terminal ticket event and a done status event.
func TestCompleteRun_PublishesEvents(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeRunHandler(st, eventsService)

	payload := map[string]any{
		"run_id": runID.String(),
		"status": "succeeded",
		"stats":  map[string]any{"exit_code": 0},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify events were published to the hub by checking the snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) < 2 {
		t.Fatalf("expected at least 2 events (ticket + done), got %d", len(snapshot))
	}

	// Verify we have both a ticket event and a done event.
	foundTicketEvent := false
	foundDoneEvent := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicketEvent = true
			// Verify the event contains "succeeded" status.
			if !strings.Contains(string(evt.Data), "succeeded") {
				t.Errorf("expected ticket event data to contain 'succeeded', got: %s", string(evt.Data))
			}
		}
		if evt.Type == "done" {
			foundDoneEvent = true
			// Verify the event contains done status.
			if !strings.Contains(string(evt.Data), "done") {
				t.Errorf("expected done event data to contain 'done', got: %s", string(evt.Data))
			}
		}
	}
	if !foundTicketEvent {
		t.Error("expected to find a 'ticket' event in the snapshot")
	}
	if !foundDoneEvent {
		t.Error("expected to find a 'done' event in the snapshot")
	}
}
