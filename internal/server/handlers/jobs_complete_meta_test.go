package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

const testLogDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// ===== JobMeta Validation Tests =====
// These tests verify that job_meta payloads are validated via contracts.UnmarshalJobMeta
// before persisting to jobs.meta JSONB.

// TestCompleteJob_InvalidJobMeta_MissingKind returns 400 when job_meta lacks required kind field.
func TestCompleteJob_InvalidJobMeta_MissingKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// job_meta without required "kind" field should be rejected.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"gate": map[string]any{"log_digest": testLogDigest},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 400 Bad Request for invalid job_meta (missing kind field).
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "job_meta") {
		t.Errorf("expected error message to mention job_meta, got: %s", rr.Body.String())
	}
	// Verify job completion was NOT called.
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect job completion to be called for invalid job_meta")
	}
}

// TestCompleteJob_InvalidJobMeta_InvalidKind returns 400 when job_meta has unrecognized kind.
func TestCompleteJob_InvalidJobMeta_InvalidKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// job_meta with invalid "kind" value should be rejected.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "invalid_kind",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 400 Bad Request for invalid kind.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "job_meta") {
		t.Errorf("expected error message to mention job_meta, got: %s", rr.Body.String())
	}
	// Verify job completion was NOT called.
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect job completion to be called for invalid job_meta")
	}
}

// TestCompleteJob_InvalidJobMeta_GateMetaOnModKind returns 400 when job_meta has
// gate metadata but kind is "mod" (structural mismatch).
func TestCompleteJob_InvalidJobMeta_GateMetaOnModKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// job_meta with kind="mod" but gate metadata should be rejected (structural mismatch).
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "mod",
				"gate": map[string]any{"log_digest": testLogDigest},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 400 Bad Request for structural mismatch.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify job completion was NOT called.
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect job completion to be called for invalid job_meta")
	}
}

// TestCompleteJob_ValidJobMeta_GateKind verifies that valid gate job_meta is accepted and persisted.
func TestCompleteJob_ValidJobMeta_GateKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Valid gate job_meta with proper kind and gate metadata.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": testLogDigest,
					"static_checks": []map[string]any{
						{"tool": "maven", "passed": true},
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 204 No Content for valid job_meta.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletionWithMeta was called (not UpdateJobCompletion).
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called when meta is provided")
	}
	// Validate the persisted meta contains expected kind.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "gate" {
		t.Fatalf("expected meta.kind == \"gate\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_ValidJobMeta_ModKind verifies that valid mod job_meta is accepted.
func TestCompleteJob_ValidJobMeta_ModKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Valid mod job_meta (kind only, no gate/build metadata).
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "mod",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 204 No Content for valid mod job_meta.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletionWithMeta was called.
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Validate the persisted meta contains expected kind.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "mod" {
		t.Fatalf("expected meta.kind == \"mod\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_ValidJobMeta_BuildKind verifies that valid build job_meta is accepted.
func TestCompleteJob_ValidJobMeta_BuildKind(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1500,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Valid build job_meta with kind and build metadata.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "build",
				"build": map[string]any{
					"tool":    "maven",
					"command": "mvn clean install",
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 204 No Content for valid build job_meta.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletionWithMeta was called.
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Validate the persisted meta contains expected kind.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "build" {
		t.Fatalf("expected meta.kind == \"build\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_EmptyJobMeta_NoPersist verifies empty job_meta values don't trigger
// UpdateJobCompletionWithMeta (use regular UpdateJobCompletion instead).
func TestCompleteJob_EmptyJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Empty job_meta object should NOT trigger UpdateJobCompletionWithMeta.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    map[string]any{}, // Empty object
			"duration_ms": 500,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletion was called (not WithMeta variant).
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for empty job_meta")
	}
}

// TestCompleteJob_NullJobMeta_NoPersist verifies null job_meta values don't trigger
// UpdateJobCompletionWithMeta.
func TestCompleteJob_NullJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Null job_meta should NOT trigger UpdateJobCompletionWithMeta.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    nil,
			"duration_ms": 500,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Expect 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletion was called (not WithMeta variant).
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for null job_meta")
	}
}

// TestCompleteJob_ValidJobMeta_GateWithBugSummary verifies that gate job_meta with
// bug_summary is accepted and persisted.
func TestCompleteJob_ValidJobMeta_GateWithBugSummary(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Gate job_meta with bug_summary in gate metadata.
	body, _ := json.Marshal(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest":  testLogDigest,
					"bug_summary": "Missing semicolon on line 42 of Main.java",
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Verify bug_summary is persisted in gate metadata.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	gate, ok := meta["gate"].(map[string]any)
	if !ok {
		t.Fatal("expected gate metadata to be present")
	}
	if bs, ok := gate["bug_summary"].(string); !ok || bs != "Missing semicolon on line 42 of Main.java" {
		t.Fatalf("expected bug_summary = %q, got %#v", "Missing semicolon on line 42 of Main.java", gate["bug_summary"])
	}
}

// TestCompleteJob_ValidJobMeta_ModWithActionSummary verifies that mod job_meta
// with action_summary is accepted and persisted.
func TestCompleteJob_ValidJobMeta_ModWithActionSummary(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID,
		RunID:     runID,
		NodeID:    &nodeID,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Mod job_meta with action_summary.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind":           "mod",
				"action_summary": "Fixed missing import in Main.java",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Verify action_summary is persisted in meta.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if as, ok := meta["action_summary"].(string); !ok || as != "Fixed missing import in Main.java" {
		t.Fatalf("expected action_summary = %q, got %#v", "Fixed missing import in Main.java", meta["action_summary"])
	}
}

// ===== JobStatsPayload Unit Tests =====
// These tests verify the typed JobStatsPayload struct behavior.

func TestJobStatsPayload_MRURL(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected string
	}{
		{
			name:     "nil metadata",
			payload:  JobStatsPayload{},
			expected: "",
		},
		{
			name:     "empty metadata",
			payload:  JobStatsPayload{Metadata: map[string]string{}},
			expected: "",
		},
		{
			name:     "mr_url present",
			payload:  JobStatsPayload{Metadata: map[string]string{"mr_url": "https://gitlab.com/mr/1"}},
			expected: "https://gitlab.com/mr/1",
		},
		{
			name:     "mr_url with whitespace",
			payload:  JobStatsPayload{Metadata: map[string]string{"mr_url": "  https://gitlab.com/mr/2  "}},
			expected: "https://gitlab.com/mr/2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.payload.MRURL()
			if got != tc.expected {
				t.Errorf("MRURL() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestJobStatsPayload_HasJobMeta(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected bool
	}{
		{
			name:     "nil job_meta",
			payload:  JobStatsPayload{},
			expected: false,
		},
		{
			name:     "empty job_meta bytes",
			payload:  JobStatsPayload{JobMeta: []byte{}},
			expected: false,
		},
		{
			name:     "empty object job_meta",
			payload:  JobStatsPayload{JobMeta: []byte("{}")},
			expected: false,
		},
		{
			name:     "empty object job_meta with whitespace",
			payload:  JobStatsPayload{JobMeta: []byte("{ }")},
			expected: false,
		},
		{
			name:     "null job_meta",
			payload:  JobStatsPayload{JobMeta: []byte("null")},
			expected: false,
		},
		{
			name:     "valid job_meta",
			payload:  JobStatsPayload{JobMeta: []byte(`{"kind":"mod"}`)},
			expected: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.payload.HasJobMeta()
			if got != tc.expected {
				t.Errorf("HasJobMeta() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestJobStatsPayload_ValidateJobMeta(t *testing.T) {
	tests := []struct {
		name    string
		payload JobStatsPayload
		wantErr bool
	}{
		{
			name:    "nil job_meta",
			payload: JobStatsPayload{},
			wantErr: false, // No job_meta is valid (optional).
		},
		{
			name:    "empty job_meta",
			payload: JobStatsPayload{JobMeta: []byte("{}")},
			wantErr: false, // Empty is treated as "no job_meta".
		},
		{
			name:    "empty job_meta with whitespace",
			payload: JobStatsPayload{JobMeta: []byte("{ }")},
			wantErr: false, // Empty is treated as "no job_meta".
		},
		{
			name:    "null job_meta",
			payload: JobStatsPayload{JobMeta: []byte("null")},
			wantErr: false, // Null is treated as "no job_meta".
		},
		{
			name:    "valid mod kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"mod"}`)},
			wantErr: false,
		},
		{
			name:    "valid gate kind",
			payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"gate\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: false,
		},
		{
			name:    "valid build kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"build","build":{"tool":"maven"}}`)},
			wantErr: false,
		},
		{
			name:    "missing kind field",
			payload: JobStatsPayload{JobMeta: []byte("{\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: true,
		},
		{
			name:    "invalid kind value",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"unknown"}`)},
			wantErr: true,
		},
		{
			name:    "gate metadata on mod kind",
			payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"mod\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: true,
		},
		{
			name:    "invalid json",
			payload: JobStatsPayload{JobMeta: []byte(`{invalid}`)},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.payload.ValidateJobMeta()
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateJobMeta() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
