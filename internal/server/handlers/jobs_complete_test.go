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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Request body only contains status (no run_id, job_id, or step_index needed).
	body, _ := json.Marshal(map[string]any{
		"status": "succeeded",
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	exitCode := int32(0)
	body, _ := json.Marshal(map[string]any{
		"status":    "succeeded",
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 9000,
		ModType:   "mr",
	}

	mrURL := "https://gitlab.com/org/repo/-/merge_requests/42"

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusSucceeded,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "succeeded",
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	// Embed JobMeta-shaped payload under stats.job_meta.
	body, _ := json.Marshal(map[string]any{
		"status":    "succeeded",
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

// TestCompleteJob_MissingJobID returns 400 when job_id is not in the path.
func TestCompleteJob_MissingJobID(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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
		RunID:     runID,
		NodeID:    &otherNodeStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusPending, // Not 'running'
		StepIndex: 1000,
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "failed"})
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
		RunID:     runID,
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
		"status": "failed",
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

	body, _ := json.Marshal(map[string]any{"status": "failed"})
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

	job := store.Job{
		ID:        jobID.String(),
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID.String(),
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeJobHandler(st, eventsService)

	body, _ := json.Marshal(map[string]any{
		"status":    "succeeded",
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	nextJob := store.Job{
		ID:        nextJobID.String(),
		RunID:     runID,
		Status:    store.JobStatusCreated,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:          job,
		listJobsByRunResult:   []store.Job{job, nextJob},
		scheduleNextJobResult: nextJob,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "failed"})
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
	modJobID := domaintypes.NewJobID()
	postJobID := domaintypes.NewJobID()

	// Jobs: pre-gate succeeded, mod failed, post-gate created.
	jobs := []store.Job{
		{
			ID:        domaintypes.NewJobID().String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Status:    store.JobStatusSucceeded,
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        modJobID.String(),
			RunID:     runID,
			NodeID:    &nodeIDStr,
			Status:    store.JobStatusRunning,
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
		{
			ID:        postJobID.String(),
			RunID:     runID,
			Status:    store.JobStatusCreated,
			StepIndex: 3000,
			Meta:      []byte(`{"mod_type":"post_gate"}`),
		},
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        jobs[1], // mod job
		listJobsByRunResult: jobs,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{
		"status":    "failed",
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
			if call.Status != store.JobStatusCanceled {
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
		RunID:     runID,
		NodeID:    &nodeIDStr,
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "canceled"})
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
	if st.updateJobCompletionParams.Status != store.JobStatusCanceled {
		t.Fatalf("expected job status canceled, got %s", st.updateJobCompletionParams.Status)
	}
}
