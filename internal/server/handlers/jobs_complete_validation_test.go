package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Input Validation & Auth Tests =====
// These tests verify request validation and authorization for completeJobHandler.

func TestCompleteJob_BadJobID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		jobID string
	}{
		{name: "missing", jobID: ""},
		{name: "whitespace", jobID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nodeID := domaintypes.NewNodeKey()
			handler := completeJobHandler(&mockStore{}, nil, nil)

			body, _ := json.Marshal(map[string]any{"status": "Success"})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
			req.SetPathValue("job_id", tt.jobID)
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
		})
	}
}

func TestCompleteJob_NoIdentity(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	handler := completeJobHandler(&mockStore{}, nil, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

func TestCompleteJob_BadNodeHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setHeader  bool
		headerVal  string
		commonName string
	}{
		{name: "empty", setHeader: true, headerVal: "   ", commonName: "ignored"},
		{name: "invalid_format", setHeader: true, headerVal: "not-a-nanoid", commonName: "ignored"},
		{name: "missing", setHeader: false, headerVal: "", commonName: "tok_abcdef123456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			st := &mockStore{
				getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil, nil)

			body, _ := json.Marshal(map[string]any{"status": "Success"})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
			req.SetPathValue("job_id", f.JobID.String())
			if tt.setHeader {
				req.Header.Set(nodeUUIDHeader, tt.headerVal)
			}
			ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
				Role:       auth.RoleWorker,
				CommonName: tt.commonName,
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
		})
	}
}

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
		Status: domaintypes.JobStatusRunning,
		Meta:   withNextIDMeta([]byte(`{}`), 1000),
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil, nil)

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

func TestCompleteJob_NonRunningConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jobStatus domaintypes.JobStatus
	}{
		{name: "created", jobStatus: domaintypes.JobStatusCreated},
		{name: "success", jobStatus: domaintypes.JobStatusSuccess},
		{name: "fail", jobStatus: domaintypes.JobStatusFail},
		{name: "cancelled", jobStatus: domaintypes.JobStatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			f.Job.Status = tt.jobStatus

			st := &mockStore{
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil, nil)
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

func TestCompleteJob_InvalidStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "running"}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestCompleteJob_InvalidRepoSHAOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
	}{
		{name: "uppercase", sha: "0123456789ABCDEF0123456789ABCDEF01234567"},
		{name: "too_short", sha: "0123456789abcdef"},
		{name: "non_hex", sha: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			st := &mockStore{
				getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":       "Success",
				"repo_sha_out": tt.sha,
			}))

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
			}
			if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
				t.Fatal("did not expect completion persistence on invalid repo_sha_out")
			}
		})
	}
}

func TestCompleteJob_MissingRepoSHAOutForSuccessfulLinkedJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	nextID := domaintypes.NewJobID()
	f.Job.NextID = &nextID
	f.Job.RepoShaIn = "0123456789abcdef0123456789abcdef01234567"

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
	}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect completion persistence when repo_sha_out is missing")
	}
}

func TestCompleteJob_InvalidRepoSHAInForSuccessfulLinkedJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	nextID := domaintypes.NewJobID()
	f.Job.NextID = &nextID
	f.Job.RepoShaIn = ""

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect completion persistence when repo_sha_in is invalid")
	}
}

func TestCompleteJob_MissingStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestCompleteJob_StatsMustBeObject(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
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

func TestCompleteJob_JobNotFound(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{getJobErr: pgx.ErrNoRows}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestCompleteJob_InvalidJobResources_RejectsRequest(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
		"stats": map[string]any{
			"job_resources": map[string]any{
				"cpu_consumed_ns": -1,
			},
		},
	}))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect job completion updates on invalid job_resources")
	}
	if st.upsertJobMetricCalled {
		t.Fatal("did not expect UpsertJobMetric on invalid job_resources")
	}
}
