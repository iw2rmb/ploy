package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	// Since createStageParams is now a slice, check the first element.
	if len(st.createStageParams) == 0 {
		t.Fatal("expected at least one CreateStage call")
	}
	if st.createStageParams[0].RunID.Bytes != runID {
		t.Errorf("CreateStage called with wrong run_id: %v", st.createStageParams[0].RunID)
	}
	if st.createStageParams[0].Name != "mods-openrewrite" {
		t.Errorf("expected stage name 'mods-openrewrite', got %s", st.createStageParams[0].Name)
	}
	if st.createStageParams[0].Status != store.StageStatusPending {
		t.Errorf("expected stage status %s, got %s", store.StageStatusPending, st.createStageParams[0].Status)
	}
}

// ===== Step-Level Claim Tests =====
// These tests verify the step-level claiming mechanism for multi-step runs,
// where nodes claim individual steps rather than entire runs.

// TestClaimRun_StepLevelClaim_Success verifies that a node successfully claims
// a single step from a multi-step run via ClaimRunStep, and receives a response
// with step_index populated.
func TestClaimRun_StepLevelClaim_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	stageID := uuid.New()
	now := time.Now()
	stepIndex := int32(0)

	// Mock store that returns a node, a claimed step (via ClaimRunStep),
	// and the parent run metadata.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		// ClaimRunStep returns a RunStep with step_index=0.
		claimRunStepResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: stepID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: stepIndex,
			Status:    store.RunStepStatusAssigned,
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		// GetRun provides parent run metadata for step-level claim response.
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusQueued, // Run status may still be queued; step is assigned.
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"steps":[{"action":"shell","cmd":"echo step0"},{"action":"shell","cmd":"echo step1"}]}`),
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

	// Verify ClaimRunStep was called with the correct node ID.
	if !st.claimRunStepCalled {
		t.Fatal("expected ClaimRunStep to be called")
	}
	if st.claimRunStepParams.Bytes != nodeID {
		t.Fatalf("ClaimRunStep called with wrong node id: %v", st.claimRunStepParams)
	}

	// Verify GetRun was called to fetch parent run metadata.
	if !st.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
	if st.getRunParams.Bytes != runID {
		t.Fatalf("GetRun called with wrong run id: %v", st.getRunParams)
	}

	// Parse response and verify step-level claim details.
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify run_id is present and matches the parent run.
	if resp["id"] != runID.String() {
		t.Errorf("expected id %s, got %v", runID.String(), resp["id"])
	}

	// Verify step_index is present in the response.
	stepIndexFloat, ok := resp["step_index"].(float64)
	if !ok {
		t.Fatalf("expected step_index to be present and numeric, got %v", resp["step_index"])
	}
	if int32(stepIndexFloat) != stepIndex {
		t.Errorf("expected step_index %d, got %d", stepIndex, int32(stepIndexFloat))
	}

	// Verify node_id matches the claiming node.
	if resp["node_id"] != nodeID.String() {
		t.Errorf("expected node_id %s, got %v", nodeID.String(), resp["node_id"])
	}

	// Verify run metadata is included (repo_url, base_ref, target_ref).
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url from parent run, got %v", resp["repo_url"])
	}
	if resp["base_ref"] != "main" {
		t.Errorf("expected base_ref from parent run, got %v", resp["base_ref"])
	}
	if resp["target_ref"] != "feature-branch" {
		t.Errorf("expected target_ref from parent run, got %v", resp["target_ref"])
	}
}

// TestClaimRun_StepLevelClaim_FallbackToWholRun verifies that when ClaimRunStep
// returns ErrNoRows (no steps available), the handler falls back to ClaimRun
// to claim a whole single-step run.
func TestClaimRun_StepLevelClaim_FallbackToWholeRun(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	stageID := uuid.New()
	now := time.Now()

	// Mock store where ClaimRunStep returns ErrNoRows (no steps available),
	// but ClaimRun succeeds (single-step run available).
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		// ClaimRunStep returns ErrNoRows → trigger fallback to ClaimRun.
		claimRunStepErr: pgx.ErrNoRows,
		// ClaimRun succeeds with a single-step run.
		claimRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusAssigned,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"action":"shell","cmd":"echo hello"}`),
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

	// Verify ClaimRunStep was called first.
	if !st.claimRunStepCalled {
		t.Fatal("expected ClaimRunStep to be called first")
	}

	// Verify ClaimRun was called as fallback.
	if !st.claimRunCalled {
		t.Fatal("expected ClaimRun to be called as fallback")
	}

	// Parse response and verify step_index is NOT present (whole-run claim).
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// step_index should be absent for whole-run claims.
	if _, exists := resp["step_index"]; exists {
		t.Errorf("expected step_index to be absent for whole-run claim, got %v", resp["step_index"])
	}

	// Verify run metadata is present.
	if resp["id"] != runID.String() {
		t.Errorf("expected id %s, got %v", runID.String(), resp["id"])
	}
	if resp["status"] != string(store.RunStatusAssigned) {
		t.Errorf("expected status %s, got %v", store.RunStatusAssigned, resp["status"])
	}
}

// TestClaimRun_StepLevelClaim_BothStrategiesFail verifies that when both
// ClaimRunStep and ClaimRun return ErrNoRows, the handler returns 204 No Content.
func TestClaimRun_StepLevelClaim_BothStrategiesFail(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()

	// Mock store where both ClaimRunStep and ClaimRun return ErrNoRows.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimRunStepErr: pgx.ErrNoRows,
		claimRunErr:     pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimRunHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 204 No Content (no work available).
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify both claiming strategies were attempted.
	if !st.claimRunStepCalled {
		t.Fatal("expected ClaimRunStep to be called")
	}
	if !st.claimRunCalled {
		t.Fatal("expected ClaimRun to be called as fallback")
	}
}

// TestClaimRun_StepLevelClaim_MultipleSteps verifies that distinct nodes can
// claim different steps of the same multi-step run sequentially.
func TestClaimRun_StepLevelClaim_MultipleSteps(t *testing.T) {
	t.Parallel()

	node1ID := uuid.New()
	node2ID := uuid.New()
	runID := uuid.New()
	step0ID := uuid.New()
	step1ID := uuid.New()
	stageID := uuid.New()
	now := time.Now()

	// Scenario: node1 claims step 0, then node2 claims step 1.
	// We'll simulate two separate requests.

	// ===== Node 1 claims step 0 =====
	st1 := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: node1ID, Valid: true},
		},
		claimRunStepResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: step0ID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: 0,
			Status:    store.RunStepStatusAssigned,
			NodeID:    pgtype.UUID{Bytes: node1ID, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			Status:    store.RunStatusQueued,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"steps":[{"action":"shell","cmd":"echo step0"},{"action":"shell","cmd":"echo step1"}]}`),
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

	configHolder := &ConfigHolder{}
	handler1 := claimRunHandler(st1, configHolder)

	req1 := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+node1ID.String()+"/claim", nil)
	req1.SetPathValue("id", node1ID.String())
	rr1 := httptest.NewRecorder()

	handler1.ServeHTTP(rr1, req1)

	// Verify node1 successfully claimed step 0.
	if rr1.Code != http.StatusOK {
		t.Fatalf("node1 claim: expected status 200, got %d: %s", rr1.Code, rr1.Body.String())
	}

	var resp1 map[string]any
	if err := json.NewDecoder(rr1.Body).Decode(&resp1); err != nil {
		t.Fatalf("node1 claim: failed to decode response: %v", err)
	}

	if resp1["id"] != runID.String() {
		t.Errorf("node1 claim: expected run id %s, got %v", runID.String(), resp1["id"])
	}
	stepIndex1, ok := resp1["step_index"].(float64)
	if !ok || int32(stepIndex1) != 0 {
		t.Errorf("node1 claim: expected step_index 0, got %v", resp1["step_index"])
	}
	if resp1["node_id"] != node1ID.String() {
		t.Errorf("node1 claim: expected node_id %s, got %v", node1ID.String(), resp1["node_id"])
	}

	// ===== Node 2 claims step 1 =====
	st2 := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: node2ID, Valid: true},
		},
		claimRunStepResult: store.RunStep{
			ID:        pgtype.UUID{Bytes: step1ID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			StepIndex: 1,
			Status:    store.RunStepStatusAssigned,
			NodeID:    pgtype.UUID{Bytes: node2ID, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			Status:    store.RunStatusRunning, // Run is now running after step 0 started.
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"steps":[{"action":"shell","cmd":"echo step0"},{"action":"shell","cmd":"echo step1"}]}`),
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

	handler2 := claimRunHandler(st2, configHolder)

	req2 := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+node2ID.String()+"/claim", nil)
	req2.SetPathValue("id", node2ID.String())
	rr2 := httptest.NewRecorder()

	handler2.ServeHTTP(rr2, req2)

	// Verify node2 successfully claimed step 1.
	if rr2.Code != http.StatusOK {
		t.Fatalf("node2 claim: expected status 200, got %d: %s", rr2.Code, rr2.Body.String())
	}

	var resp2 map[string]any
	if err := json.NewDecoder(rr2.Body).Decode(&resp2); err != nil {
		t.Fatalf("node2 claim: failed to decode response: %v", err)
	}

	if resp2["id"] != runID.String() {
		t.Errorf("node2 claim: expected run id %s, got %v", runID.String(), resp2["id"])
	}
	stepIndex2, ok := resp2["step_index"].(float64)
	if !ok || int32(stepIndex2) != 1 {
		t.Errorf("node2 claim: expected step_index 1, got %v", resp2["step_index"])
	}
	if resp2["node_id"] != node2ID.String() {
		t.Errorf("node2 claim: expected node_id %s, got %v", node2ID.String(), resp2["node_id"])
	}
}
