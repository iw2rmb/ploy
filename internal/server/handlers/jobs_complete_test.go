package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Job-Level Completion Tests =====
// completeJobHandler marks a job as completed via POST /v1/jobs/{job_id}/complete.
// This endpoint simplifies the node → server contract by addressing jobs directly.

// TestCompleteJob_Success verifies a job is completed successfully with valid payload.
// Node identity is derived from mTLS context; job_id is in the URL path.
func TestCompleteJob_Success(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	// Set up mock to return job via GetJob.
	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Name:      "mod-0",
		Status:    store.JobStatusRunning,
		ModType:   "mod",
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Request body only contains status (no run_id, job_id, or step_index needed).
	body, _ := json.Marshal(map[string]any{
		"status": "Success",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	// Inject node identity into context (simulates mTLS authentication).
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify GetJob was called to look up the job.
	if !st.getJobCalled {
		t.Fatal("expected GetJob to be called")
	}
	// Verify UpdateJobCompletion was called.
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_WithExitCodeAndStats verifies job completion with exit_code and stats.
func TestCompleteJob_WithExitCodeAndStats(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Name:      "mod-0",
		Status:    store.JobStatusRunning,
		ModType:   "mod",
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	exitCode := int32(0)
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": exitCode,
		"stats":     map[string]any{"duration_ms": 1234},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify exit_code was passed to UpdateJobCompletion.
	if st.updateJobCompletionParams.ExitCode == nil {
		t.Fatal("expected exit_code to be set")
	}
	if *st.updateJobCompletionParams.ExitCode != exitCode {
		t.Fatalf("expected exit_code %d, got %d", exitCode, *st.updateJobCompletionParams.ExitCode)
	}
}

// TestCompleteJob_MRJobUpdatesRunStatsMRURL verifies that when an MR job
// completes with stats.metadata.mr_url, the handler merges that URL into
// runs.stats via UpdateRunStatsMRURL.
func TestCompleteJob_MRJobUpdatesRunStatsMRURL(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 9000,
		ModType:   "mr",
	}

	mrURL := "https://gitlab.com/org/repo/-/merge_requests/42"

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusFinished,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"metadata": map[string]any{
				"mr_url": mrURL,
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.updateRunStatsMRURLCalled {
		t.Fatal("expected UpdateRunStatsMRURL to be called")
	}
	if st.updateRunStatsMRURLParams.ID != runID.String() {
		t.Fatalf("expected UpdateRunStatsMRURL run_id %s, got %s", runID, st.updateRunStatsMRURLParams.ID)
	}
	if st.updateRunStatsMRURLParams.MrUrl != mrURL {
		t.Fatalf("expected UpdateRunStatsMRURL mr_url %q, got %q", mrURL, st.updateRunStatsMRURLParams.MrUrl)
	}
}

// TestCompleteJob_WithJobMetaInStats verifies that when stats.job_meta is provided,
// the handler uses UpdateJobCompletionWithMeta to persist jobs.meta JSONB.
func TestCompleteJob_WithJobMetaInStats(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Embed JobMeta-shaped payload under stats.job_meta.
	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": "sha256:test",
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// When job_meta is present, handler should prefer UpdateJobCompletionWithMeta.
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called when meta is provided")
	}

	// Validate that persisted meta JSON contains the expected kind.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "gate" {
		t.Fatalf("expected meta.kind == \"gate\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored verifies that an empty
// job_meta object (even if it contains whitespace like "{ }") is treated as absent
// and does not cause a 400 nor trigger jobs.meta persistence.
func TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// NOTE: Do not use json.Marshal here; we need whitespace inside job_meta ("{ }").
	body := `{"status":"Success","exit_code":0,"stats":{"duration_ms":500,"job_meta": { } }}`

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader([]byte(body)))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta to be called")
	}
}

// TestCompleteJob_MissingJobID returns 400 when job_id is not in the path.
func TestCompleteJob_MissingJobID(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "") // Empty job_id
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_EmptyJobID returns 400 for empty job_id.
// Job IDs are now KSUID strings; only empty/whitespace IDs are rejected.
func TestCompleteJob_EmptyJobID(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	// Note: "not-a-uuid" is now a valid KSUID string ID, so we only test empty ID.
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "   ") // Whitespace ID
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_NoIdentity returns 401 when no identity is in context.
func TestCompleteJob_NoIdentity(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	// No identity injected into context.

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

// TestCompleteJob_EmptyNodeHeader returns 400 when PLOY_NODE_UUID header is empty.
// Node IDs are now NanoID(6) strings; only empty/whitespace IDs are rejected.
func TestCompleteJob_EmptyNodeHeader(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	// Note: "not-a-uuid" is now a valid NanoID string ID, so we test empty header.
	req.Header.Set(nodeUUIDHeader, "   ") // Whitespace header

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: "ignored",
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_MissingNodeHeader returns 400 when PLOY_NODE_UUID header is missing.
func TestCompleteJob_MissingNodeHeader(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())

	// Simulate bearer-token identity: CommonName is a non-UUID token id.
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: "tok_abcdef123456", // not a UUID, no "node:" prefix
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_WrongNode returns 403 when job is assigned to a different node.
func TestCompleteJob_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	otherNode := domaintypes.NewNodeKey()
	otherNodeStr := otherNode
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	// Job is assigned to a different node (otherNode).
	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &otherNodeStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID, // Different from job's node
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_NotRunning returns 409 when the job is not in running state.
func TestCompleteJob_NotRunning(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	// Job is in 'pending' status (not 'running').
	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusCreated, // Not 'running'
		StepIndex: 1000,
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Fail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_InvalidStatus returns 400 when non-terminal status provided.
func TestCompleteJob_InvalidStatus(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	jobID := domaintypes.NewJobID()

	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "running"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_MissingStatus returns 400 when status is not provided.
func TestCompleteJob_MissingStatus(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	jobID := domaintypes.NewJobID()

	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_StatsMustBeObject returns 400 when stats is not a JSON object.
func TestCompleteJob_StatsMustBeObject(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// stats provided as a string, which is valid JSON but not an object.
	body, _ := json.Marshal(map[string]any{
		"status": "Fail",
		"stats":  "oops",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_JobNotFound returns 404 when job does not exist.
func TestCompleteJob_JobNotFound(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	jobID := domaintypes.NewJobID()

	st := &mockStore{
		getJobErr: pgx.ErrNoRows,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Fail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

// TestCompleteJob_PublishesEvents verifies that completing a job publishes events
// when the run transitions to terminal state.
func TestCompleteJob_PublishesEvents(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	now := time.Now()

	repoID := domaintypes.NewModRepoID().String()

	job := store.Job{
		ID:          jobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "mod-0",
		Status:      store.JobStatusRunning,
		ModType:     "mod",
		StepIndex:   1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID.String(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
		// v1: repo-scoped progression requires CountJobsByRunRepoAttemptGroupByStatus
		// to return all jobs terminal for repo status update to occur.
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{
			{Status: store.JobStatusSuccess, Count: 1},
		},
		// All repos terminal triggers run completion.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeJobHandler(st, eventsService)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats":     map[string]any{"duration_ms": 500},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify events were published to the hub.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) < 2 {
		t.Fatalf("expected at least 2 events (run + done), got %d", len(snapshot))
	}

	// Verify we have both a run summary event and a done event.
	foundRunEvent := false
	foundDoneEvent := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRunEvent = true
			if !strings.Contains(string(evt.Data), "succeeded") {
				t.Errorf("expected run event data to contain 'succeeded', got: %s", string(evt.Data))
			}
		}
		if evt.Type == "done" {
			foundDoneEvent = true
		}
	}
	if !foundRunEvent {
		t.Error("expected to find a 'run' event in the snapshot")
	}
	if !foundDoneEvent {
		t.Error("expected to find a 'done' event in the snapshot")
	}
}

// TestCompleteJob_SchedulesNextJob verifies that a successful job completion
// triggers scheduling of the next job.
func TestCompleteJob_SchedulesNextJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nextJobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	nextJob := store.Job{
		ID:        nextJobID.String(),
		RunID:     runID.String(),
		Status:    store.JobStatusCreated,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:          job,
		listJobsByRunResult:   []store.Job{job, nextJob},
		scheduleNextJobResult: nextJob,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ScheduleNextJob was called.
	if !st.scheduleNextJobCalled {
		t.Fatal("expected ScheduleNextJob to be called")
	}
}

// TestCompleteJob_FailedJobDoesNotScheduleNext verifies that a failed job
// does not trigger scheduling of the next job.
func TestCompleteJob_FailedJobDoesNotScheduleNext(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Fail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ScheduleNextJob was NOT called for failed jobs.
	if st.scheduleNextJobCalled {
		t.Fatal("did not expect ScheduleNextJob to be called for failed job")
	}
}

// TestCompleteJob_ModFailureCancelsRemainingJobs verifies that when a non-gate
// mod job fails, remaining non-terminal jobs are canceled so the run can
// transition to a terminal state instead of leaving jobs stranded.
func TestCompleteJob_ModFailureCancelsRemainingJobs(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID().String()
	modJobID := domaintypes.NewJobID()
	postJobID := domaintypes.NewJobID()

	// Jobs: pre-gate succeeded, mod failed, post-gate created.
	jobs := []store.Job{
		{
			ID:          domaintypes.NewJobID().String(),
			RunID:       runID.String(),
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeIDStr,
			Status:      store.JobStatusSuccess,
			ModType:     domaintypes.ModTypePreGate.String(),
			StepIndex:   1000,
			Meta:        []byte(`{}`),
		},
		{
			ID:          modJobID.String(),
			RunID:       runID.String(),
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeIDStr,
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMod.String(),
			StepIndex:   2000,
			Meta:        []byte(`{}`),
		},
		{
			ID:          postJobID.String(),
			RunID:       runID.String(),
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			Status:      store.JobStatusCreated,
			ModType:     domaintypes.ModTypePostGate.String(),
			StepIndex:   3000,
			Meta:        []byte(`{}`),
		},
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:                   jobs[1], // mod job
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+modJobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", modJobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify UpdateJobCompletion was called for the mod job.
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.ID != jobs[1].ID {
		t.Fatalf("expected UpdateJobCompletion for mod job, got %v", st.updateJobCompletionParams.ID)
	}

	// Verify UpdateJobStatus was called to cancel the post-gate job.
	if !st.updateJobStatusCalled {
		t.Fatal("expected UpdateJobStatus to be called to cancel remaining jobs")
	}
	if len(st.updateJobStatusCalls) == 0 {
		t.Fatal("expected at least one UpdateJobStatus call")
	}
	foundPostCancel := false
	for _, call := range st.updateJobStatusCalls {
		if call.ID == jobs[2].ID {
			foundPostCancel = true
			if call.Status != store.JobStatusCancelled {
				t.Fatalf("expected post-gate job to be canceled, got status %s", call.Status)
			}
		}
	}
	if !foundPostCancel {
		t.Fatal("expected post-gate job to be canceled")
	}
}

// TestCompleteJob_CanceledStatus verifies that canceled status is accepted.
func TestCompleteJob_CanceledStatus(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Cancelled"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.Status != store.JobStatusCancelled {
		t.Fatalf("expected job status canceled, got %s", st.updateJobCompletionParams.Status)
	}
}

// ===== JobMeta Validation Tests =====
// These tests verify that job_meta payloads are validated via contracts.UnmarshalJobMeta
// before persisting to jobs.meta JSONB.

// TestCompleteJob_InvalidJobMeta_MissingKind returns 400 when job_meta lacks required kind field.
func TestCompleteJob_InvalidJobMeta_MissingKind(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
				"gate": map[string]any{"log_digest": "sha256:abc"},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
				"gate": map[string]any{"log_digest": "sha256:abc"},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
					"log_digest": "sha256:abc123",
					"static_checks": []map[string]any{
						{"tool": "maven", "passed": true},
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1500,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID.String(),
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
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
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
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
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"gate","gate":{"log_digest":"sha256:abc"}}`)},
			wantErr: false,
		},
		{
			name:    "valid build kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"build","build":{"tool":"maven"}}`)},
			wantErr: false,
		},
		{
			name:    "missing kind field",
			payload: JobStatsPayload{JobMeta: []byte(`{"gate":{"log_digest":"sha256:abc"}}`)},
			wantErr: true,
		},
		{
			name:    "invalid kind value",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"unknown"}`)},
			wantErr: true,
		},
		{
			name:    "gate metadata on mod kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"mod","gate":{"log_digest":"sha256:abc"}}`)},
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

// ===== Repo-Scoped Status Progression Tests =====
// These tests verify v1 repo-scoped progression per roadmap/v1/scope.md:98 and roadmap/v1/statuses.md:193.
// When a job completes:
// - run_repos.status is updated when all jobs for the repo attempt are terminal
// - runs.status becomes Finished when all repos are terminal

// TestCompleteJob_RepoStatusUpdatedOnLastJob verifies that run_repos.status is updated
// to Success when the last job in a repo attempt completes successfully.
func TestCompleteJob_RepoStatusUpdatedOnLastJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	repoID := domaintypes.NewModRepoID().String()

	// Single job per repo, completing it should mark repo as terminal.
	job := store.Job{
		ID:          jobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "mod-0",
		Status:      store.JobStatusRunning,
		ModType:     "mod",
		StepIndex:   2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
		// Return that all jobs (1 total) are now Success after completion.
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{
			{Status: store.JobStatusSuccess, Count: 1},
		},
		// All repos terminal (1 Success), so run becomes Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify CountJobsByRunRepoAttemptGroupByStatus was called to check repo terminal state.
	if !st.countJobsByRunRepoAttemptGroupByStatusCalled {
		t.Fatal("expected CountJobsByRunRepoAttemptGroupByStatus to be called")
	}
	if st.countJobsByRunRepoAttemptGroupByStatusParams.RunID != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID, st.countJobsByRunRepoAttemptGroupByStatusParams.RunID)
	}
	if st.countJobsByRunRepoAttemptGroupByStatusParams.RepoID != repoID {
		t.Errorf("expected repo_id %s, got %s", repoID, st.countJobsByRunRepoAttemptGroupByStatusParams.RepoID)
	}

	// Verify UpdateRunRepoStatus was called to update repo to Success.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != store.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}

	// Verify UpdateRunStatus was called to set run to Finished.
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.Status != store.RunStatusFinished {
		t.Errorf("expected run status Finished, got %s", st.updateRunStatusParams.Status)
	}
}

// TestCompleteJob_RepoStatusFail verifies that run_repos.status is updated
// to Fail when a job in the repo attempt fails.
func TestCompleteJob_RepoStatusFail(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	repoID := domaintypes.NewModRepoID().String()

	// Job that will fail.
	job := store.Job{
		ID:          jobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "mod-0",
		Status:      store.JobStatusRunning,
		ModType:     "mod",
		StepIndex:   2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:                   job,
		listJobsByRunResult:            []store.Job{job},
		listJobsByRunRepoAttemptResult: []store.Job{job},
		// All jobs terminal: 1 Fail.
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{
			{Status: store.JobStatusFail, Count: 1},
		},
		// All repos terminal.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusFail, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo status was updated to Fail.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != store.RunRepoStatusFail {
		t.Errorf("expected repo status Fail, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_RepoNotTerminalWhileJobsInProgress verifies that run_repos.status
// is NOT updated when there are still non-terminal jobs for the repo attempt.
func TestCompleteJob_RepoNotTerminalWhileJobsInProgress(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nextJobID := domaintypes.NewJobID()
	repoID := domaintypes.NewModRepoID().String()

	// Two jobs: first completes, second is still Created.
	job1 := store.Job{
		ID:          jobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "pre-gate",
		Status:      store.JobStatusRunning,
		ModType:     "pre_gate",
		StepIndex:   1000,
	}
	job2 := store.Job{
		ID:          nextJobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mod-0",
		Status:      store.JobStatusCreated,
		ModType:     "mod",
		StepIndex:   2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:          job1,
		listJobsByRunResult:   []store.Job{job1, job2},
		scheduleNextJobResult: job2,
		// 1 Success, 1 Created => not all terminal.
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{
			{Status: store.JobStatusSuccess, Count: 1},
			{Status: store.JobStatusCreated, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo status was NOT updated (jobs still in progress).
	if st.updateRunRepoStatusCalled {
		t.Error("did not expect UpdateRunRepoStatus to be called while jobs still in progress")
	}

	// Verify run status was NOT updated to Finished.
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called while repo not terminal")
	}

	// Verify next job was scheduled.
	if !st.scheduleNextJobCalled {
		t.Fatal("expected ScheduleNextJob to be called")
	}
}

// TestCompleteJob_MRJobDoesNotAffectRepoStatus verifies that MR jobs (mod_type='mr')
// do NOT trigger repo status updates, per roadmap/v1/statuses.md:77.
func TestCompleteJob_MRJobDoesNotAffectRepoStatus(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	repoID := domaintypes.NewModRepoID().String()

	// MR job (auxiliary, should not affect repo/run status).
	mrJob := store.Job{
		ID:          jobID.String(),
		RunID:       runID.String(),
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "mr-0",
		Status:      store.JobStatusRunning,
		ModType:     "mr", // MR job type
		StepIndex:   9000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusFinished, // MR jobs run after run is Finished.
		},
		getJobResult:        mrJob,
		listJobsByRunResult: []store.Job{mrJob},
		// Should NOT be called for MR jobs.
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify CountJobsByRunRepoAttemptGroupByStatus was NOT called for MR jobs.
	if st.countJobsByRunRepoAttemptGroupByStatusCalled {
		t.Error("did not expect CountJobsByRunRepoAttemptGroupByStatus to be called for MR job")
	}

	// Verify repo status was NOT updated.
	if st.updateRunRepoStatusCalled {
		t.Error("did not expect UpdateRunRepoStatus to be called for MR job")
	}

	// Verify run status was NOT updated (already Finished, MR doesn't change it).
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called for MR job")
	}
}

// TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal verifies that runs.status
// becomes Finished only when ALL repos reach terminal state, not just one.
func TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	nodeIDStr := nodeID
	runID := domaintypes.NewRunID()
	jobIDRepoA := domaintypes.NewJobID()
	repoIDA := domaintypes.NewModRepoID().String()
	// repoIDB is another repo in the run, still Running (not explicitly used but modeled in countRunReposByStatusResult).
	_ = domaintypes.NewModRepoID().String() // repoIDB - unused but conceptually part of the multi-repo scenario

	// Job for repo A completing (repo B still has work).
	jobRepoA := store.Job{
		ID:          jobIDRepoA.String(),
		RunID:       runID.String(),
		RepoID:      repoIDA,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeIDStr,
		Name:        "mod-0",
		Status:      store.JobStatusRunning,
		ModType:     "mod",
		StepIndex:   2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusStarted,
		},
		getJobResult:        jobRepoA,
		listJobsByRunResult: []store.Job{jobRepoA},
		// Repo A is now terminal (all jobs Success).
		countJobsByRunRepoAttemptGroupByStatusResult: []store.CountJobsByRunRepoAttemptGroupByStatusRow{
			{Status: store.JobStatusSuccess, Count: 1},
		},
		// But repo B is still Running, so run should NOT become Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1}, // Repo A
			{Status: store.RunRepoStatusRunning, Count: 1}, // Repo B still running
		},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobIDRepoA.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobIDRepoA.String())
	req.Header.Set(nodeUUIDHeader, nodeID)

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo A status was updated to Success.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called for repo A")
	}

	// Verify run status was NOT updated to Finished (repo B still in progress).
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called when not all repos are terminal")
	}
}
