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
		Status:  domaintypes.JobStatusRunning,
		JobType: "mig",
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

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobImageName.called {
		t.Fatalf("expected UpdateJobImageName to be called")
	}
	if st.updateJobImageName.params.ID != jobID {
		t.Fatalf("UpdateJobImageName ID = %s, want %s", st.updateJobImageName.params.ID, jobID)
	}
	if st.updateJobImageName.params.JobImage != "docker.io/example/migs:latest" {
		t.Fatalf("UpdateJobImageName JobImage = %q, want %q", st.updateJobImageName.params.JobImage, "docker.io/example/migs:latest")
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

	assertStatus(t, rr, http.StatusBadRequest)
	if st.updateJobImageName.called {
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
		Status:  domaintypes.JobStatusRunning,
		JobType: "mig",
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

	assertStatus(t, rr, http.StatusForbidden)
	if st.updateJobImageName.called {
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
		Status:  domaintypes.JobStatusQueued,
		JobType: "mig",
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

	assertStatus(t, rr, http.StatusConflict)
	if st.updateJobImageName.called {
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
		Status:  domaintypes.JobStatusRunning,
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

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobImageName.called {
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
		Status:  domaintypes.JobStatusRunning,
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

	assertStatus(t, rr, http.StatusConflict)
	if st.updateJobImageName.called {
		t.Fatalf("expected UpdateJobImageName NOT to be called")
	}
}
