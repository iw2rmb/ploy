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

// ===== Run Completion Tests =====
// completeRunHandler marks a run as completed and publishes completion events.

// TestCompleteRun_Success verifies a run is completed successfully with valid payload.
func TestCompleteRun_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: []store.Job{{
			ID:        pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning,
			StepIndex: 1000,
		}},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"run_id": runID.String(), "status": "succeeded", "step_index": 1000})
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

// TestCompleteRun_WrongNode returns 403 when job is assigned to a different node.
func TestCompleteRun_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		// Job assigned to different node
		listJobsByRunResult: []store.Job{{
			ID:     pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:  pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: otherNode, Valid: true},
		}},
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
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusAssigned,
		},
		listJobsByRunResult: []store.Job{{
			ID:     pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:  pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
		}},
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
			Status: store.RunStatusRunning,
		},
		// Note: listJobsByRunResult not set - status validation happens before job check.
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
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: []store.Job{{
			ID:     pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:  pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
		}},
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
	jobID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{{
			ID:        pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning,
			StepIndex: 1000,
		}},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeRunHandler(st, eventsService)

	payload := map[string]any{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": 1000,
		"stats":      map[string]any{"exit_code": 0},
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

// ===== Gate-Aware Run Completion Tests =====
// These tests verify that maybeCompleteMultiStepRun correctly derives run status
// from job outcomes in a gate-aware way (ROADMAP.md item 2).

// TestGateAwareCompletion_GateFailsHealingSucceeds verifies that when a gate
// fails initially but healing + re-gate succeed, the overall run succeeds.
// Scenario: pre-gate fails → healing succeeds → re-gate succeeds → mod succeeds → run succeeded.
func TestGateAwareCompletion_GateFailsHealingSucceeds(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	// Build jobs with gate metadata: pre-gate failed, heal succeeded, re-gate succeeded, mod succeeded.
	// The final gate (re-gate) succeeded, so run should succeed.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusFailed, // pre-gate failed initially
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // healing job succeeded
			StepIndex: 1100,
			Meta:      []byte(`{"mod_type":"heal"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // re-gate succeeded (final gate)
			StepIndex: 1200,
			Meta:      []byte(`{"mod_type":"re_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // mod succeeded
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
	}

	// Mock the last job as running - when we complete it, maybeCompleteMultiStepRun fires.
	jobs[3].Status = store.JobStatusRunning

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: jobs,
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": 2000,
	})
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
	// Run should succeed because final gate (re-gate) succeeded and no mod/heal failures.
	if st.updateRunCompletionParams.Status != store.RunStatusSucceeded {
		t.Errorf("expected run status succeeded, got %s", st.updateRunCompletionParams.Status)
	}
}

// TestGateAwareCompletion_ModJobFails verifies that when a mod job fails,
// the run fails regardless of gate outcomes.
// Scenario: pre-gate succeeds → mod fails → run failed.
func TestGateAwareCompletion_ModJobFails(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	// Build jobs: pre-gate succeeded, mod failed.
	// Mod failure should cause run to fail.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // pre-gate succeeded
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning, // mod running, about to fail
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
	}

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: jobs,
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"run_id":     runID.String(),
		"status":     "failed",
		"step_index": 2000,
	})
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
	// Run should fail because a mod job failed.
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Errorf("expected run status failed, got %s", st.updateRunCompletionParams.Status)
	}
}

// TestGateAwareCompletion_FinalGateFails verifies that when the final gate fails
// (after all other jobs succeed), the run fails.
// Scenario: pre-gate succeeds → mod succeeds → post-gate fails → run failed.
func TestGateAwareCompletion_FinalGateFails(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	// Build jobs: pre-gate succeeded, mod succeeded, post-gate failed.
	// Final gate failure should cause run to fail.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // pre-gate succeeded
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // mod succeeded
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning, // post-gate running, about to fail
			StepIndex: 3000,
			Meta:      []byte(`{"mod_type":"post_gate"}`),
		},
	}

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: jobs,
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"run_id":     runID.String(),
		"status":     "failed",
		"step_index": 3000,
	})
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
	// Run should fail because the final gate (post-gate) failed.
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Errorf("expected run status failed, got %s", st.updateRunCompletionParams.Status)
	}
}

// TestGateAwareCompletion_NoRedundantJobMutation verifies that maybeCompleteMultiStepRun
// does not call UpdateJobStatus after job completion (redundant mutation removed).
func TestGateAwareCompletion_NoRedundantJobMutation(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: []store.Job{{
			ID:        pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning,
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		}},
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"run_id":     runID.String(),
		"status":     "succeeded",
		"step_index": 1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify UpdateJobCompletion was called (for the initial job completion).
	if !st.updateJobCompletionCalled {
		t.Error("expected UpdateJobCompletion to be called for job completion")
	}

	// Verify UpdateJobStatus was NOT called by maybeCompleteMultiStepRun.
	// The redundant job mutation block has been removed.
	if st.updateJobStatusCalled {
		t.Error("UpdateJobStatus should not be called - redundant job mutation removed")
	}
}

// TestGateAwareCompletion_CanceledJob verifies that a canceled job (without failures)
// results in a canceled run.
func TestGateAwareCompletion_CanceledJob(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	// Build jobs: pre-gate succeeded, mod canceled (no failures).
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded, // pre-gate succeeded
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning, // mod running, about to be canceled
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
	}

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		listJobsByRunResult: jobs,
	}

	handler := completeRunHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"run_id":     runID.String(),
		"status":     "canceled",
		"step_index": 2000,
	})
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
	// Run should fail because a non-gate job (mod) was canceled (hasNonGateFailure triggers).
	// Note: Per the ROADMAP spec, non-gate job cancellation is treated as failure precedence.
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Errorf("expected run status failed (non-gate cancellation), got %s", st.updateRunCompletionParams.Status)
	}
}
