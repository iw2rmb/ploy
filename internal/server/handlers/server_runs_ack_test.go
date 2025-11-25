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

// ===== Run Acknowledgement Tests =====
// ackRunStartHandler acknowledges that a node has started working on an assigned run.

// TestAckRunStart_Success verifies 204 and store transition when the run
// is assigned to the requesting node.
func TestAckRunStart_Success(t *testing.T) {
	t.Parallel()

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

	handler := ackRunStartHandler(st, nil)

	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called")
	}
	if st.ackRunStartParam.Bytes != runID {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}

// TestAckRunStart_WrongNode verifies 403 when the run is assigned to a different node.
func TestAckRunStart_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: otherNode, Valid: true},
			Status: store.RunStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.ackRunStartCalled {
		t.Fatal("did not expect AckRunStart to be called")
	}
}

// TestAckRunStart_WrongStatus verifies 409 when the run is not in assigned state.
func TestAckRunStart_WrongStatus(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning, // not assigned
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.ackRunStartCalled {
		t.Fatal("did not expect AckRunStart to be called")
	}
}

// TestAckRunStart_PublishesEvent verifies that acknowledging a run publishes a running event.
func TestAckRunStart_PublishesEvent(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusAssigned,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := ackRunStartHandler(st, eventsService)

	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify a ticket event was published to the hub by checking the snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least one ticket event to be published")
	}

	// Verify the event type is "ticket".
	foundTicketEvent := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicketEvent = true
			// Verify the event contains ticket state information with "running" status.
			if !strings.Contains(string(evt.Data), "running") {
				t.Errorf("expected ticket event data to contain 'running', got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundTicketEvent {
		t.Error("expected to find a 'ticket' event in the snapshot")
	}
}

// ===== Run Step Acknowledgement Tests =====
// These tests verify step-level ack flow for multi-step runs.

// TestAckRunStepStart_Success verifies 204 and AckRunStepStart is called
// when a valid step_index is provided and the step is assigned to the node.
func TestAckRunStepStart_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(1)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusQueued, // Run status doesn't matter for step-level ack
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st, nil)

	// Include step_index in the payload to trigger step-level ack.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify GetRunStepByIndex was called with correct params.
	if !st.getRunStepByIndexCalled {
		t.Fatal("expected GetRunStepByIndex to be called")
	}
	if st.getRunStepByIndexParams.RunID.Bytes != runID {
		t.Fatalf("GetRunStepByIndex called with wrong run_id: %v", st.getRunStepByIndexParams.RunID)
	}
	if st.getRunStepByIndexParams.StepIndex != stepIndex {
		t.Fatalf("GetRunStepByIndex called with wrong step_index: %d", st.getRunStepByIndexParams.StepIndex)
	}

	// Verify AckRunStepStart was called with the step ID.
	if !st.ackRunStepStartCalled {
		t.Fatal("expected AckRunStepStart to be called")
	}
	if st.ackRunStepStartParam.Bytes != stepID {
		t.Fatalf("AckRunStepStart called with wrong step id: %v", st.ackRunStepStartParam)
	}

	// Verify run-level ack was also called to transition run status to 'running'.
	// This is the new behavior: when a step starts, the run also transitions to 'running'.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called for step-level ack")
	}
	if st.ackRunStartParam.Bytes != runID {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}

// TestAckRunStepStart_WrongNode verifies 403 when the step is assigned to a different node.
func TestAckRunStepStart_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(0)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusQueued,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: otherNode, Valid: true}, // Different node
			Status:    store.RunStepStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.ackRunStepStartCalled {
		t.Fatal("did not expect AckRunStepStart to be called")
	}
}

// TestAckRunStepStart_WrongStatus verifies 409 when the step is not in assigned state.
func TestAckRunStepStart_WrongStatus(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(2)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusQueued,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning, // Already running, not assigned
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.ackRunStepStartCalled {
		t.Fatal("did not expect AckRunStepStart to be called")
	}
}

// TestAckRunStepStart_StepNotFound verifies 404 when the step doesn't exist.
func TestAckRunStepStart_StepNotFound(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepIndex := int32(5)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusQueued,
		},
		getRunStepByIndexErr: pgx.ErrNoRows, // Step not found
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if st.ackRunStepStartCalled {
		t.Fatal("did not expect AckRunStepStart to be called")
	}
}

// ===== Run-level Start Semantics for Multi-step Runs =====
// These tests verify that run.status transitions to 'running' when the first step starts.

// TestAckRunStepStart_FirstStepTransitionsRunToRunning verifies that when the first step
// of a multi-step run is acknowledged, both the step and the run status transition to 'running'.
func TestAckRunStepStart_FirstStepTransitionsRunToRunning(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(0) // First step

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusQueued, // Multi-step run starts in 'queued' status
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step-level ack was called.
	if !st.ackRunStepStartCalled {
		t.Fatal("expected AckRunStepStart to be called")
	}
	if st.ackRunStepStartParam.Bytes != stepID {
		t.Fatalf("AckRunStepStart called with wrong step id: %v", st.ackRunStepStartParam)
	}

	// Verify run-level ack was also called to transition run status to 'running'.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called for first step of multi-step run")
	}
	if st.ackRunStartParam.Bytes != runID {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}

// TestAckRunStepStart_SubsequentStepLeavesRunStatusUnchanged verifies that when a
// subsequent step (not the first) is acknowledged, only the step status transitions to 'running'
// and the run status is left unchanged (already 'running' from first step ack).
func TestAckRunStepStart_SubsequentStepLeavesRunStatusUnchanged(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(2) // Subsequent step (not first)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning, // Run already in 'running' status from first step
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step-level ack was called.
	if !st.ackRunStepStartCalled {
		t.Fatal("expected AckRunStepStart to be called")
	}

	// Verify run-level ack was still called (it's a no-op for already 'running' status).
	// The SQL query won't match any rows since status is 'running', not 'queued' or 'assigned'.
	// This is acceptable behavior - AckRunStart is idempotent.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called (even though it's a no-op for subsequent steps)")
	}
}
