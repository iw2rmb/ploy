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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	// Set up mock to return job via GetJob.
	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	// Inject node identity into context (simulates mTLS authentication).
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

// TestCompleteJob_MissingJobID returns 400 when job_id is not in the path.
func TestCompleteJob_MissingJobID(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "") // Empty job_id
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_InvalidJobID returns 400 for malformed UUID.
func TestCompleteJob_InvalidJobID(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/not-a-uuid/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "not-a-uuid")
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	jobID := uuid.New()
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

// TestCompleteJob_InvalidNodeHeader returns 400 when PLOY_NODE_UUID header is not a valid UUID.
func TestCompleteJob_InvalidNodeHeader(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "succeeded"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, "not-a-uuid")

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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
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

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	// Job is assigned to a different node (otherNode).
	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: otherNode, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(), // Different from job's node
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	// Job is in 'pending' status (not 'running').
	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "running"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{
		getJobErr: pgx.ErrNoRows,
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "failed"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	now := time.Now()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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
		t.Fatalf("expected at least 2 events (ticket + done), got %d", len(snapshot))
	}

	// Verify we have both a ticket event and a done event.
	foundTicketEvent := false
	foundDoneEvent := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicketEvent = true
			if !strings.Contains(string(evt.Data), "succeeded") {
				t.Errorf("expected ticket event data to contain 'succeeded', got: %s", string(evt.Data))
			}
		}
		if evt.Type == "done" {
			foundDoneEvent = true
		}
	}
	if !foundTicketEvent {
		t.Error("expected to find a 'ticket' event in the snapshot")
	}
	if !foundDoneEvent {
		t.Error("expected to find a 'done' event in the snapshot")
	}
}

// TestCompleteJob_SchedulesNextJob verifies that a successful job completion
// triggers scheduling of the next job.
func TestCompleteJob_SchedulesNextJob(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	nextJobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	nextJob := store.Job{
		ID:        pgtype.UUID{Bytes: nextJobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		Status:    store.JobStatusCreated,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "failed"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	modJobID := uuid.New()
	postJobID := uuid.New()

	// Jobs: pre-gate succeeded, mod failed, post-gate created.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusSucceeded,
			StepIndex: 1000,
			Meta:      []byte(`{"mod_type":"pre_gate"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: modJobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
			Status:    store.JobStatusRunning,
			StepIndex: 2000,
			Meta:      []byte(`{"mod_type":"mod"}`),
		},
		{
			ID:        pgtype.UUID{Bytes: postJobID, Valid: true},
			RunID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status:    store.JobStatusCreated,
			StepIndex: 3000,
			Meta:      []byte(`{"mod_type":"post_gate"}`),
		},
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
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
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	job := store.Job{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		Status:    store.JobStatusRunning,
		StepIndex: 1000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			Status: store.RunStatusRunning,
		},
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "canceled"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: nodeID.String(),
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
