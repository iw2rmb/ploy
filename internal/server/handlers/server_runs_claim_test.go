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

// ===== Job Claim Tests =====
// claimJobHandler allows nodes to claim a pending job for execution.

// TestClaimJob_Success verifies a node successfully claims a pending job
// when a job is available and the node exists.
func TestClaimJob_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	now := time.Now()

	// Mock store that returns a node, a claimed job, and the parent run.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimJobResult: store.Job{
			ID:        pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Name:      "mod-0",
			Status:    store.JobStatusRunning, // Jobs go directly to running on claim
			StepIndex: 2000,
			Meta:      []byte("{}"),
		},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"key":"value"}`),
		},
	}

	// Create handler with empty config holder.
	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 200 OK.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimJob was called with the correct node ID.
	if !st.claimJobCalled {
		t.Fatal("expected ClaimJob to be called")
	}
	if st.claimJobParams.Bytes != nodeID {
		t.Fatalf("ClaimJob called with wrong node id: %v", st.claimJobParams)
	}

	// Verify GetRun was called to fetch parent run metadata.
	if !st.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}

	// Parse response and verify job details.
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["id"] != runID.String() {
		t.Errorf("expected id (run_id) %s, got %v", runID.String(), resp["id"])
	}
	if resp["job_id"] != jobID.String() {
		t.Errorf("expected job_id %s, got %v", jobID.String(), resp["job_id"])
	}
	if resp["job_name"] != "mod-0" {
		t.Errorf("expected job_name 'mod-0', got %v", resp["job_name"])
	}
	stepIndex, ok := resp["step_index"].(float64)
	if !ok || stepIndex != 2000 {
		t.Errorf("expected step_index 2000, got %v", resp["step_index"])
	}
	if resp["node_id"] != nodeID.String() {
		t.Errorf("expected node_id %s, got %v", nodeID.String(), resp["node_id"])
	}
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url from parent run, got %v", resp["repo_url"])
	}

	// Verify that spec was enriched with job_id and mod_index for the mod job.
	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	if spec["job_id"] != jobID.String() {
		t.Errorf("expected spec.job_id %s, got %v", jobID.String(), spec["job_id"])
	}
	// Numbers decode as float64 in generic maps.
	if mi, ok := spec["mod_index"].(float64); !ok || mi != 0 {
		t.Errorf("expected spec.mod_index 0, got %v", spec["mod_index"])
	}
}

// TestClaimJob_NoJobsAvailable verifies 204 No Content is returned when
// no pending jobs are available for claiming.
func TestClaimJob_NoJobsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()

	// Mock store that returns a node but no available jobs (ClaimJob returns ErrNoRows).
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimJobErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimJob was called.
	if !st.claimJobCalled {
		t.Fatal("expected ClaimJob to be called")
	}
}

// TestClaimJob_NodeNotFound verifies 404 Not Found is returned when
// the requesting node does not exist.
func TestClaimJob_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()

	// Mock store that returns ErrNoRows for GetNode.
	st := &mockStore{
		getNodeErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 404 Not Found.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ClaimJob was not called since node check failed.
	if st.claimJobCalled {
		t.Fatal("did not expect ClaimJob to be called")
	}
}

// TestClaimJob_InvalidNodeID verifies 400 Bad Request is returned when
// the node ID path parameter is not a valid UUID.
func TestClaimJob_InvalidNodeID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/invalid-uuid/claim", nil)
	req.SetPathValue("id", "invalid-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 400 Bad Request.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestClaimJob_AcksRunStart verifies that claiming a job on a queued run
// transitions the run to running status via AckRunStart.
func TestClaimJob_AcksRunStart(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	now := time.Now()

	// Mock store with a queued run - should trigger AckRunStart.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: pgtype.UUID{Bytes: nodeID, Valid: true},
		},
		claimJobResult: store.Job{
			ID:        pgtype.UUID{Bytes: jobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Name:      "pre-gate",
			Status:    store.JobStatusRunning, // Jobs go directly to running on claim
			StepIndex: 1000,
			Meta:      []byte("{}"),
		},
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.RunStatusQueued, // Still queued
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{}`),
		},
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 200 OK.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify AckRunStart was called to transition run to running.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called for queued run")
	}
	if st.ackRunStartParam.Bytes != runID {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}
