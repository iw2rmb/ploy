package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestSaveJobImageName_Success(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		NodeID:  &nodeID,
		Status:  store.JobStatusRunning,
		JobType: "mod",
	}

	st := &mockStore{
		getJobResult: job,
	}

	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{
		"image": "docker.io/example/migs:latest",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName to be called")
	}
	if st.updateJobImageNameParams.ID != jobID {
		t.Fatalf("UpdateJobImageName ID = %s, want %s", st.updateJobImageNameParams.ID, jobID)
	}
	if st.updateJobImageNameParams.JobImage != "docker.io/example/migs:latest" {
		t.Fatalf("UpdateJobImageName JobImage = %q, want %q", st.updateJobImageNameParams.JobImage, "docker.io/example/migs:latest")
	}
}

func TestSaveJobImageName_EmptyImage(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	jobID := domaintypes.NewJobID()

	st := &mockStore{}
	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{
		"image": "   ",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName NOT to be called")
	}
}

func TestSaveJobImageName_ForbiddenWrongNode(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	otherNode := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		NodeID:  &otherNode,
		Status:  store.JobStatusRunning,
		JobType: "mod",
	}

	st := &mockStore{
		getJobResult: job,
	}
	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{"image": "docker.io/example/migs:latest"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName NOT to be called")
	}
	_ = nodeID // avoid unused in case of future refactors
}

func TestSaveJobImageName_ConflictJobNotRunning(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		NodeID:  &nodeID,
		Status:  store.JobStatusQueued,
		JobType: "mod",
	}

	st := &mockStore{getJobResult: job}
	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{"image": "docker.io/example/migs:latest"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName NOT to be called")
	}
}

func TestSaveJobImageName_SuccessGateJob(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		NodeID:  &nodeID,
		Status:  store.JobStatusRunning,
		JobType: "pre_gate",
	}

	st := &mockStore{getJobResult: job}
	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{"image": "docker.io/example/migs:latest"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName to be called")
	}
}

func TestSaveJobImageName_ConflictWrongJobType(t *testing.T) {
	t.Parallel()

	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:      jobID,
		RunID:   runID,
		NodeID:  &nodeID,
		Status:  store.JobStatusRunning,
		JobType: "mr",
	}

	st := &mockStore{getJobResult: job}
	handler := saveJobImageNameHandler(st)

	body, _ := json.Marshal(map[string]any{"image": "docker.io/example/migs:latest"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobImageNameCalled {
		t.Fatalf("expected UpdateJobImageName NOT to be called")
	}
}
