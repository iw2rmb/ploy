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

// ===== Run Claim Tests =====
// claimRunHandler allows nodes to claim a queued run for execution.

// TestClaimRun_Success verifies a node successfully claims a queued run
// when a run is available and the node exists.
func TestClaimRun_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stageID := uuid.New()
	now := time.Now()

	// Mock store that returns a node, a claimed run, and a stage for that run.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusAssigned,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"key":"value"}`),
		},
		listStagesByRunResult: []store.Stage{
			{
				ID:     pgtype.UUID{Bytes: stageID, Valid: true},
				RunID:  pgtype.UUID{Bytes: runID, Valid: true},
				Name:   "mods-openrewrite",
				Status: store.StageStatusPending,
				Meta:   []byte("{}"),
			},
		},
	}

	// Create handler with empty config holder.
	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 200 OK.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimRun was called with the correct node ID.
	if !st.claimRunCalled {
		t.Fatal("expected ClaimRun to be called")
	}
	if st.claimRunParams.Bytes != nodeID {
		t.Fatalf("ClaimRun called with wrong node id: %v", st.claimRunParams)
	}

	// Parse response and verify run details.
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["id"] != runID.String() {
		t.Errorf("expected id %s, got %v", runID.String(), resp["id"])
	}
	if resp["status"] != string(store.RunStatusAssigned) {
		t.Errorf("expected status %s, got %v", store.RunStatusAssigned, resp["status"])
	}
	if resp["node_id"] != nodeID.String() {
		t.Errorf("expected node_id %s, got %v", nodeID.String(), resp["node_id"])
	}
}

// TestClaimRun_NoRunsAvailable verifies 204 No Content is returned when
// no queued runs are available for claiming.
func TestClaimRun_NoRunsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()

	// Mock store that returns a node but no available runs (ClaimRun returns ErrNoRows).
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimRunErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimRun was called.
	if !st.claimRunCalled {
		t.Fatal("expected ClaimRun to be called")
	}
}

// TestClaimRun_NodeNotFound verifies 404 Not Found is returned when
// the requesting node does not exist.
func TestClaimRun_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()

	// Mock store that returns ErrNoRows for GetNode.
	st := &mockStore{
		getNodeErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 404 Not Found.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimRun was not called since node check failed.
	if st.claimRunCalled {
		t.Fatal("did not expect ClaimRun to be called")
	}
}

// TestClaimRun_InvalidNodeID verifies 400 Bad Request is returned when
// the node ID path parameter is not a valid UUID.
func TestClaimRun_InvalidNodeID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/invalid-uuid/claim", nil)
	req.SetPathValue("id", "invalid-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 400 Bad Request.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestClaimRun_CreatesStageWhenNoneExist verifies that a stage is created
// when a run is claimed but no stages exist for that run.
func TestClaimRun_CreatesStageWhenNoneExist(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	now := time.Now()

	// Mock store that returns no stages, so a new stage should be created.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusAssigned,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{}`),
		},
		listStagesByRunResult: []store.Stage{}, // No stages exist
	}

	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 200 OK.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify CreateStage was called.
	if !st.createStageCalled {
		t.Fatal("expected CreateStage to be called")
	}

	// Verify the stage was created with correct parameters.
	if st.createStageParams.RunID.Bytes != runID {
		t.Errorf("CreateStage called with wrong run_id: %v", st.createStageParams.RunID)
	}
	if st.createStageParams.Name != "mods-openrewrite" {
		t.Errorf("expected stage name 'mods-openrewrite', got %s", st.createStageParams.Name)
	}
	if st.createStageParams.Status != store.StageStatusPending {
		t.Errorf("expected stage status %s, got %s", store.StageStatusPending, st.createStageParams.Status)
	}
}

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

// ===== Run Completion Tests =====
// completeRunHandler allows nodes to mark a running run as completed (succeeded/failed).

// TestCompleteRun_Success verifies a node can complete a running run it owns
// and that UpdateRunCompletion is invoked with the expected parameters.
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
	t.Parallel()

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

	// Parse and verify response.
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

// TestGetRunTiming_NotFound verifies 404 is returned when the run does not exist.
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

// TestGetRunTiming_InvalidID verifies 400 is returned for an invalid run ID.
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
