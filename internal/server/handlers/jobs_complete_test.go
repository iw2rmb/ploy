package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Job-Level Completion Tests =====
// completeJobHandler marks a job as completed via POST /v1/jobs/{job_id}/complete.
// This endpoint simplifies the node → server contract by addressing jobs directly.

// TestCompleteJob_Success verifies a job is completed successfully with valid payload.
func TestCompleteJob_Success(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Success"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response body, got %q", rr.Body.String())
	}
	if !st.getJobCalled {
		t.Fatal("expected GetJob to be called")
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_WithExitCodeAndStats verifies job completion with exit_code and stats.
func TestCompleteJob_WithExitCodeAndStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": int32(0),
		"stats":     map[string]any{"duration_ms": 1234},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionParams.ExitCode == nil {
		t.Fatal("expected exit_code to be set")
	}
	if *st.updateJobCompletionParams.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d", *st.updateJobCompletionParams.ExitCode)
	}
}

// TestCompleteJob_MRJobUpdatesRunStatsMRURL verifies that when an MR job
// completes with stats.metadata.mr_url, the handler merges that URL into
// runs.stats via UpdateRunStatsMRURL.
func TestCompleteJob_MRJobUpdatesRunStatsMRURL(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mr", 9000)
	mrURL := "https://gitlab.com/org/repo/-/merge_requests/42"

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusFinished},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"metadata":    map[string]any{"mr_url": mrURL},
		},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunStatsMRURLCalled {
		t.Fatal("expected UpdateRunStatsMRURL to be called")
	}
	if st.updateRunStatsMRURLParams.ID != f.RunID {
		t.Fatalf("expected UpdateRunStatsMRURL run_id %s, got %s", f.RunID, st.updateRunStatsMRURLParams.ID)
	}
	if st.updateRunStatsMRURLParams.MrUrl != mrURL {
		t.Fatalf("expected UpdateRunStatsMRURL mr_url %q, got %q", mrURL, st.updateRunStatsMRURLParams.MrUrl)
	}
}

// TestCompleteJob_WithJobMetaInStats verifies that when stats.job_meta is provided,
// the handler uses UpdateJobCompletionWithMeta to persist jobs.meta JSONB.
func TestCompleteJob_WithJobMetaInStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
		},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called when meta is provided")
	}

	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "gate" {
		t.Fatalf("expected meta.kind == \"gate\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored verifies that an empty
// job_meta object (even if it contains whitespace like "{ }") is treated as absent.
func TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	// NOTE: Do not use json.Marshal here; we need whitespace inside job_meta ("{ }").
	rawBody := `{"status":"Success","exit_code":0,"stats":{"duration_ms":500,"job_meta": { } }}`

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader([]byte(rawBody)))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: f.NodeIDStr,
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

// ===== Input Validation & Error Cases =====

// TestCompleteJob_MissingJobID returns 400 when job_id is not in the path.
func TestCompleteJob_MissingJobID(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	handler := completeJobHandler(&mockStore{}, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "")
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: nodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_EmptyJobID returns 400 for empty job_id.
func TestCompleteJob_EmptyJobID(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	handler := completeJobHandler(&mockStore{}, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
	req.SetPathValue("job_id", "   ")
	req.Header.Set(nodeUUIDHeader, nodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: nodeID,
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
	handler := completeJobHandler(&mockStore{}, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

// TestCompleteJob_EmptyNodeHeader returns 400 when PLOY_NODE_UUID header is empty.
func TestCompleteJob_EmptyNodeHeader(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, "   ")
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: "ignored",
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

func TestCompleteJob_InvalidNodeHeader(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, "not-a-nanoid")
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: "ignored",
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

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", f.JobID.String())

	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: "tok_abcdef123456",
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

	callerNodeID := domaintypes.NewNodeKey()
	ownerNodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:     jobID,
		RunID:  runID,
		NodeID: &ownerNodeID,
		Status: store.JobStatusRunning,
		Meta:   withNextIDMeta([]byte(`{}`), 1000),
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, callerNodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: callerNodeID,
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

	f := newJobFixture("mig", 1000)
	f.Job.Status = store.JobStatusCreated

	st := &mockStore{
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

func TestCompleteJob_AlreadyTerminalConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jobStatus store.JobStatus
	}{
		{name: "success", jobStatus: store.JobStatusSuccess},
		{name: "fail", jobStatus: store.JobStatusFail},
		{name: "cancelled", jobStatus: store.JobStatusCancelled},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			f.Job.Status = tt.jobStatus

			st := &mockStore{
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

			if rr.Code != http.StatusConflict {
				t.Fatalf("expected status 409, got %d", rr.Code)
			}
			if st.updateJobCompletionCalled {
				t.Fatal("did not expect UpdateJobCompletion to be called")
			}
			if st.updateJobCompletionWithMetaCalled {
				t.Fatal("did not expect UpdateJobCompletionWithMeta to be called")
			}
		})
	}
}

// TestCompleteJob_InvalidStatus returns 400 when non-terminal status provided.
func TestCompleteJob_InvalidStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "running"}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_MissingStatus returns 400 when status is not provided.
func TestCompleteJob_MissingStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

// TestCompleteJob_StatsMustBeObject returns 400 when stats is not a JSON object.
func TestCompleteJob_StatsMustBeObject(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Fail",
		"stats":  "oops",
	}))

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

	f := newJobFixture("mig", 1000)
	st := &mockStore{getJobErr: pgx.ErrNoRows}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

// TestCompleteJob_Exit137SetsLastError verifies that failed jobs with exit code
// 137 persist a deterministic run_repos.last_error message.
func TestCompleteJob_Exit137SetsLastError(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewMigRepoID()

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 137,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunRepoErrorCalled {
		t.Fatal("expected UpdateRunRepoError to be called")
	}
	if st.updateRunRepoErrorParams.RunID != f.RunID {
		t.Fatalf("expected RunID %s, got %s", f.RunID, st.updateRunRepoErrorParams.RunID)
	}
	if st.updateRunRepoErrorParams.RepoID != f.Job.RepoID {
		t.Fatalf("expected RepoID %s, got %s", f.Job.RepoID, st.updateRunRepoErrorParams.RepoID)
	}
	if st.updateRunRepoErrorParams.LastError == nil {
		t.Fatal("expected LastError to be set")
	}
	msg := *st.updateRunRepoErrorParams.LastError
	for _, want := range []string{"mig-0", "exit code 137", "out of memory"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, msg)
		}
	}
}

// TestCompleteJob_GateFailureSetsLastError verifies that when a gate job fails
// with Stack Gate mismatch metadata, the handler sets run_repos.last_error.
func TestCompleteJob_GateFailureSetsLastError(t *testing.T) {
	t.Parallel()

	f := newJobFixture("pre_gate", 1000)
	f.Job.RepoID = domaintypes.NewMigRepoID()

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: store.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"stack_gate": map[string]any{
						"enabled": true,
						"result":  "mismatch",
						"expected": map[string]any{
							"language": "java",
							"tool":     "maven",
							"release":  "17",
						},
						"detected": map[string]any{
							"language": "java",
							"tool":     "maven",
							"release":  "11",
						},
					},
					"log_findings": []map[string]any{
						{
							"severity": "error",
							"message":  "Stack mismatch",
							"evidence": "pom.xml: maven.compiler.release=11",
						},
					},
				},
			},
		},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.updateRunRepoErrorCalled {
		t.Fatal("expected UpdateRunRepoError to be called")
	}
	if st.updateRunRepoErrorParams.RunID != f.RunID {
		t.Fatalf("expected RunID %s, got %s", f.RunID, st.updateRunRepoErrorParams.RunID)
	}
	if st.updateRunRepoErrorParams.RepoID != f.Job.RepoID {
		t.Fatalf("expected RepoID %s, got %s", f.Job.RepoID, st.updateRunRepoErrorParams.RepoID)
	}
	if st.updateRunRepoErrorParams.LastError == nil {
		t.Fatal("expected LastError to be set")
	}

	errMsg := *st.updateRunRepoErrorParams.LastError
	for _, want := range []string{"inbound", "Expected:", "Detected:", `release: "17"`, `release: "11"`, "Evidence:", "pom.xml"} {
		if !strings.Contains(errMsg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, errMsg)
		}
	}
}
