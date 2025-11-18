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
