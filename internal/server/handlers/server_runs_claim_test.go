package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Job Claim Tests =====
// claimJobHandler allows nodes to claim a pending job for execution.

// TestClaimJob_Success verifies a node successfully claims a pending job
// when a job is available and the node exists.
func TestClaimJob_Success(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	now := time.Now()
	nodeIDStr := nodeID

	// Mock store that returns a node, a claimed job, and the parent run.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: nodeID,
		},
		claimJobResult: store.Job{
			ID:        jobID.String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Name:      "mod-0",
			Status:    store.JobStatusRunning, // Jobs go directly to running on claim
			StepIndex: 2000,
			Meta:      []byte("{}"),
		},
		getRunResult: store.Run{
			ID:        runID.String(),
			NodeID:    &nodeIDStr,
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{"key":"value"}`),
		},
	}

	// Create handler with empty config holder and nil events service.
	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
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
	if *st.claimJobParams != nodeID {
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
	if resp["node_id"] != nodeID {
		t.Errorf("expected node_id %s, got %v", nodeID, resp["node_id"])
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

// TestClaimJob_MergesGlobalEnvIntoSpec verifies that global environment
// variables from ConfigHolder are merged into the claimed job spec's env map
// based on scope semantics, and that per-run env values take precedence.
func TestClaimJob_MergesGlobalEnvIntoSpec(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	now := time.Now()
	nodeIDStr := nodeID

	// Run spec already contains per-run env values; these should be preserved
	// and take precedence over global env for the same keys.
	runSpec := []byte(`{"env":{"CA_CERTS_PEM_BUNDLE":"per-run-cert","PER_RUN_ONLY":"value"}}`)

	st := &mockStore{
		getNodeResult: store.Node{
			ID: nodeID,
		},
		claimJobResult: store.Job{
			ID:        jobID.String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Name:      "mod-0",
			Status:    store.JobStatusRunning,
			StepIndex: 2000,
			Meta:      []byte("{}"),
			ModType:   "mod",
		},
		getRunResult: store.Run{
			ID:        runID.String(),
			NodeID:    &nodeIDStr,
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      runSpec,
		},
	}

	// ConfigHolder contains several global env vars:
	//   - CA_CERTS_PEM_BUNDLE (scope=all)
	//   - CODEX_AUTH_JSON (scope=mods)
	//   - HEAL_ONLY (scope=heal) — should not be injected for mod jobs.
	configHolder := &ConfigHolder{}
	configHolder.SetGlobalEnvVar("CA_CERTS_PEM_BUNDLE", GlobalEnvVar{
		Value:  "global-cert",
		Scope:  domaintypes.GlobalEnvScopeAll, // Typed scope for type safety.
		Secret: true,
	})
	configHolder.SetGlobalEnvVar("CODEX_AUTH_JSON", GlobalEnvVar{
		Value:  `{"token":"xxx"}`,
		Scope:  domaintypes.GlobalEnvScopeMods, // Typed scope for type safety.
		Secret: true,
	})
	configHolder.SetGlobalEnvVar("HEAL_ONLY", GlobalEnvVar{
		Value:  "heal-env",
		Scope:  domaintypes.GlobalEnvScopeHeal, // Typed scope for type safety.
		Secret: false,
	})

	handler := claimJobHandler(st, configHolder, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}

	env, ok := spec["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.env to be an object, got %T", spec["env"])
	}

	// Per-run env value should win over global env for the same key.
	if env["CA_CERTS_PEM_BUNDLE"] != "per-run-cert" {
		t.Errorf("expected per-run CA_CERTS_PEM_BUNDLE to be preserved, got %v", env["CA_CERTS_PEM_BUNDLE"])
	}

	// New global env key with matching scope should be injected.
	if env["CODEX_AUTH_JSON"] != `{"token":"xxx"}` {
		t.Errorf("expected CODEX_AUTH_JSON to be injected from global env, got %v", env["CODEX_AUTH_JSON"])
	}

	// Global env key with non-matching scope should not be injected for mod jobs.
	if _, ok := env["HEAL_ONLY"]; ok {
		t.Errorf("expected HEAL_ONLY not to be injected for mod job, got %v", env["HEAL_ONLY"])
	}

	// Existing per-run env should be preserved.
	if env["PER_RUN_ONLY"] != "value" {
		t.Errorf("expected PER_RUN_ONLY= value, got %v", env["PER_RUN_ONLY"])
	}
}

// TestClaimJob_NoJobsAvailable verifies 204 No Content is returned when
// no pending jobs are available for claiming.
func TestClaimJob_NoJobsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()

	// Mock store that returns a node but no available jobs (ClaimJob returns ErrNoRows).
	st := &mockStore{
		getNodeResult: store.Node{
			ID: nodeID,
		},
		claimJobErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
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

	nodeID := domaintypes.NewNodeKey()

	// Mock store that returns ErrNoRows for GetNode.
	st := &mockStore{
		getNodeErr: pgx.ErrNoRows,
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
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

// TestClaimJob_EmptyNodeID verifies 400 Bad Request is returned when
// the node ID path parameter is empty or whitespace.
// Node IDs are now NanoID(6) strings; only empty/whitespace IDs are rejected.
func TestClaimJob_EmptyNodeID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder, nil)

	// Note: "invalid-uuid" is now a valid NanoID string ID, so we only test empty ID.
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes//claim", nil)
	req.SetPathValue("id", "   ") // Whitespace ID
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

	nodeID := domaintypes.NewNodeKey()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	now := time.Now()
	nodeIDStr := nodeID

	// Mock store with a queued run - should trigger AckRunStart.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: nodeID,
		},
		claimJobResult: store.Job{
			ID:        jobID.String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Name:      "pre-gate",
			Status:    store.JobStatusRunning, // Jobs go directly to running on claim
			StepIndex: 1000,
			Meta:      []byte("{}"),
		},
		getRunResult: store.Run{
			ID:        runID.String(),
			NodeID:    &nodeIDStr,
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
	handler := claimJobHandler(st, configHolder, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
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
	if st.ackRunStartParam != runID.String() {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}

// TestClaimJob_PublishesRunningEvent verifies that when claiming a job on a
// queued run, an SSE "running" event is published to notify clients in real-time.
func TestClaimJob_PublishesRunningEvent(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	now := time.Now()
	nodeIDStr := nodeID

	// Mock store with a queued run - should trigger AckRunStart and SSE event.
	st := &mockStore{
		getNodeResult: store.Node{
			ID: nodeID,
		},
		claimJobResult: store.Job{
			ID:        jobID.String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Name:      "mod-0",
			Status:    store.JobStatusRunning, // Jobs go directly to running on claim
			StepIndex: 2000,
			Meta:      []byte("{}"),
		},
		getRunResult: store.Run{
			ID:        runID.String(),
			NodeID:    &nodeIDStr,
			Status:    store.RunStatusQueued, // Still queued — triggers SSE event
			RepoUrl:   "https://github.com/user/repo.git",
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Spec:      []byte(`{}`),
		},
	}

	// Create events service for SSE fanout.
	eventsService, err := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	configHolder := &ConfigHolder{}
	handler := claimJobHandler(st, configHolder, eventsService)

	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify response status is 200 OK.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify AckRunStart was called.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called")
	}

	// Verify SSE "running" event was published by checking the hub snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least one run event to be published")
	}

	// Verify the event type is "run" and contains "running" state.
	foundRunEvent := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRunEvent = true
			if !strings.Contains(string(evt.Data), "running") {
				t.Errorf("expected run event data to contain 'running', got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundRunEvent {
		t.Error("expected to find a 'run' event in the snapshot")
	}
}
