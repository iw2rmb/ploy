package handlers

import (
	"bytes"
	"context"
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

// ===== Run Completion Tests =====
// completeRunHandler marks a run as completed and publishes completion events.

// TestCompleteRun_Success verifies a run is completed successfully with valid payload.
func TestCompleteRun_Success(t *testing.T) {
	t.Parallel()

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

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "succeeded"})
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
}

// TestCompleteRun_WrongNode returns 403 when run is assigned to a different node.
func TestCompleteRun_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: otherNode, Valid: true},
			Status: store.RunStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "succeeded"})
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

// ===== Run Step Completion Tests =====
// These tests verify step-level completion flow for multi-step runs.

// TestCompleteRunStep_Success verifies 204 and UpdateRunStepCompletion is called
// when a valid step_index is provided and the step is assigned to the node.
func TestCompleteRunStep_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(1)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	// Include step_index in the payload to trigger step-level completion.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
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

	// Verify UpdateRunStepCompletion was called with the step ID and correct status.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}
	if st.updateRunStepCompletionParams.ID.Bytes != stepID {
		t.Fatalf("UpdateRunStepCompletion called with wrong step id: %v", st.updateRunStepCompletionParams.ID)
	}
	if st.updateRunStepCompletionParams.Status != store.RunStepStatusSucceeded {
		t.Fatalf("UpdateRunStepCompletion called with wrong status: %v", st.updateRunStepCompletionParams.Status)
	}

	// Verify run-level completion was NOT called (step-level completion path).
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called for step-level completion")
	}
}

// TestCompleteRunStep_Failed verifies step completion with failed status.
func TestCompleteRunStep_Failed(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(2)
	reason := "build gate failed"

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "failed",
		"reason":     reason,
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify UpdateRunStepCompletion was called with failed status and reason.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}
	if st.updateRunStepCompletionParams.Status != store.RunStepStatusFailed {
		t.Fatalf("UpdateRunStepCompletion called with wrong status: %v", st.updateRunStepCompletionParams.Status)
	}
	if st.updateRunStepCompletionParams.Reason == nil || *st.updateRunStepCompletionParams.Reason != reason {
		t.Fatalf("UpdateRunStepCompletion called with wrong reason: %v", st.updateRunStepCompletionParams.Reason)
	}
}

// TestCompleteRunStep_WrongNode verifies 403 when the step is assigned to a different node.
func TestCompleteRunStep_WrongNode(t *testing.T) {
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
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: otherNode, Valid: true}, // Different node
			Status:    store.RunStepStatusRunning,
		},
	}

	handler := completeRunHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.updateRunStepCompletionCalled {
		t.Fatal("did not expect UpdateRunStepCompletion to be called")
	}
}

// TestCompleteRunStep_WrongStatus verifies 409 when the step is not in running state.
func TestCompleteRunStep_WrongStatus(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(1)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusSucceeded, // Already completed, not running
		},
	}

	handler := completeRunHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.updateRunStepCompletionCalled {
		t.Fatal("did not expect UpdateRunStepCompletion to be called")
	}
}

// TestCompleteRunStep_StepNotFound verifies 404 when the step doesn't exist.
func TestCompleteRunStep_StepNotFound(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepIndex := int32(5)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getRunStepByIndexErr: pgx.ErrNoRows, // Step not found
	}

	handler := completeRunHandler(st, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if st.updateRunStepCompletionCalled {
		t.Fatal("did not expect UpdateRunStepCompletion to be called")
	}
}

// ===== Multi-step Run Completion Tests =====
// These tests verify that run-level completion is derived from run_steps state
// instead of trusting the caller's status field.

// TestMultiStepRun_AllStepsSucceeded verifies the run is marked succeeded
// when all steps succeed.
func TestMultiStepRun_AllStepsSucceeded(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(2) // Last step (0, 1, 2)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
		// Multi-step run with 3 total steps.
		countRunStepsResult: 3,
	}

	// Simulate: all 3 steps succeeded (after this step completes).
	st.countRunStepsByStatusHandler = func(ctx context.Context, arg store.CountRunStepsByStatusParams) (int64, error) {
		switch arg.Status {
		case store.RunStepStatusSucceeded:
			return 3, nil // All 3 succeeded
		case store.RunStepStatusFailed:
			return 0, nil // 0 failed
		case store.RunStepStatusCanceled:
			return 0, nil // 0 canceled
		default:
			return 0, nil
		}
	}

	handler := completeRunHandler(st, nil)

	// Complete the last step as succeeded.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step completion was called.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}

	// Verify run completion was called with succeeded status.
	if !st.updateRunCompletionCalled {
		t.Fatal("expected UpdateRunCompletion to be called for multi-step run")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusSucceeded {
		t.Fatalf("expected run status succeeded, got %v", st.updateRunCompletionParams.Status)
	}
}

// TestMultiStepRun_OneStepFailed verifies the run is marked failed
// when at least one step fails.
func TestMultiStepRun_OneStepFailed(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(1) // Second step (0, 1, 2)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
		// Multi-step run with 3 total steps.
		countRunStepsResult: 3,
	}

	// Simulate: step 0 succeeded, step 1 failed (this call), step 2 canceled.
	// Use a custom handler to return different counts based on status param.
	st.countRunStepsByStatusHandler = func(ctx context.Context, arg store.CountRunStepsByStatusParams) (int64, error) {
		switch arg.Status {
		case store.RunStepStatusSucceeded:
			return 1, nil // 1 succeeded
		case store.RunStepStatusFailed:
			return 1, nil // 1 failed (after this step completes)
		case store.RunStepStatusCanceled:
			return 1, nil // 1 canceled
		default:
			return 0, nil
		}
	}

	handler := completeRunHandler(st, nil)

	// Complete the step as failed.
	reason := "build gate failed"
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "failed",
		"reason":     reason,
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step completion was called with failed status.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}
	if st.updateRunStepCompletionParams.Status != store.RunStepStatusFailed {
		t.Fatalf("expected step status failed, got %v", st.updateRunStepCompletionParams.Status)
	}

	// Verify run completion was called with failed status (derived from steps).
	if !st.updateRunCompletionCalled {
		t.Fatal("expected UpdateRunCompletion to be called for multi-step run")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Fatalf("expected run status failed, got %v", st.updateRunCompletionParams.Status)
	}
	// Verify reason includes step count.
	if st.updateRunCompletionParams.Reason == nil {
		t.Fatal("expected run completion reason to be set")
	}
	if !strings.Contains(*st.updateRunCompletionParams.Reason, "1 of 3 steps failed") {
		t.Fatalf("expected reason to contain '1 of 3 steps failed', got %s", *st.updateRunCompletionParams.Reason)
	}
}

// TestMultiStepRun_SomeStepsCanceled verifies the run is marked canceled
// when at least one step is canceled and no steps failed.
func TestMultiStepRun_SomeStepsCanceled(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(1) // Last step

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
		// Multi-step run with 2 total steps.
		countRunStepsResult: 2,
	}

	// Simulate: step 0 succeeded, step 1 canceled (this call).
	st.countRunStepsByStatusHandler = func(ctx context.Context, arg store.CountRunStepsByStatusParams) (int64, error) {
		switch arg.Status {
		case store.RunStepStatusSucceeded:
			return 1, nil // 1 succeeded
		case store.RunStepStatusFailed:
			return 0, nil // 0 failed
		case store.RunStepStatusCanceled:
			return 1, nil // 1 canceled (after this step completes)
		default:
			return 0, nil
		}
	}

	handler := completeRunHandler(st, nil)

	// Complete the step as canceled.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "canceled",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step completion was called with canceled status.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}
	if st.updateRunStepCompletionParams.Status != store.RunStepStatusCanceled {
		t.Fatalf("expected step status canceled, got %v", st.updateRunStepCompletionParams.Status)
	}

	// Verify run completion was called with canceled status (derived from steps).
	if !st.updateRunCompletionCalled {
		t.Fatal("expected UpdateRunCompletion to be called for multi-step run")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusCanceled {
		t.Fatalf("expected run status canceled, got %v", st.updateRunCompletionParams.Status)
	}
	// Verify reason includes step count.
	if st.updateRunCompletionParams.Reason == nil {
		t.Fatal("expected run completion reason to be set")
	}
	if !strings.Contains(*st.updateRunCompletionParams.Reason, "1 of 2 steps canceled") {
		t.Fatalf("expected reason to contain '1 of 2 steps canceled', got %s", *st.updateRunCompletionParams.Reason)
	}
}

// TestMultiStepRun_StillInProgress verifies the run is NOT marked complete
// when some steps are still pending/assigned/running.
func TestMultiStepRun_StillInProgress(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(0) // First step

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
		// Multi-step run with 3 total steps.
		countRunStepsResult: 3,
	}

	// Simulate: step 0 succeeded (this call), steps 1 and 2 still queued.
	st.countRunStepsByStatusHandler = func(ctx context.Context, arg store.CountRunStepsByStatusParams) (int64, error) {
		switch arg.Status {
		case store.RunStepStatusSucceeded:
			return 1, nil // 1 succeeded (after this step completes)
		case store.RunStepStatusFailed:
			return 0, nil // 0 failed
		case store.RunStepStatusCanceled:
			return 0, nil // 0 canceled
		default:
			return 0, nil
		}
	}

	handler := completeRunHandler(st, nil)

	// Complete the first step as succeeded.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step completion was called.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}

	// Verify run completion was NOT called (run still in progress).
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called (run still in progress)")
	}
}

// TestMultiStepRun_NoSteps verifies that runs without run_steps rows
// are treated as legacy single-step runs and do not invoke the multi-step logic.
func TestMultiStepRun_NoSteps(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stepIndex := int32(0)

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		getRunStepByIndexResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStepStatusRunning,
		},
		// No run_steps rows (legacy single-step run).
		countRunStepsResult: 0,
	}

	handler := completeRunHandler(st, nil)

	// Complete the step as succeeded.
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": stepIndex,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify step completion was called.
	if !st.updateRunStepCompletionCalled {
		t.Fatal("expected UpdateRunStepCompletion to be called")
	}

	// Verify run completion was NOT called (no run_steps = legacy run).
	if st.updateRunCompletionCalled {
		t.Fatal("did not expect UpdateRunCompletion to be called for legacy run")
	}
}
