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

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Job Acknowledgement Tests =====
// ackRunStartHandler acknowledges that a node has started working on an assigned job.

// TestAckJobStart_Success verifies 204 and job status transition when the job
// is assigned to the requesting node.
func TestAckJobStart_Success(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	nodeIDStr := nodeID.String()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:     runID.String(),
			NodeID: &nodeIDStr,
			Status: store.RunStatusQueued,
		},
		getJobResult: store.Job{
			ID:     jobID.String(),
			RunID:  runID.String(),
			NodeID: &nodeIDStr,
			Name:   "mod-0",
			Status: store.JobStatusRunning, // Jobs go directly to running on claim
		},
	}

	handler := ackRunStartHandler(st, nil)

	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify AckRunStart was called to transition run status.
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called")
	}
}

// TestAckJobStart_WrongNode verifies 403 when the job is assigned to a different node.
func TestAckJobStart_WrongNode(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	otherNodeStr := otherNode.String()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusQueued,
		},
		getJobResult: store.Job{
			ID:     jobID.String(),
			RunID:  runID.String(),
			NodeID: &otherNodeStr, // Different node
			Name:   "mod-0",
			Status: store.JobStatusRunning, // Jobs go directly to running on claim
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobStatusCalled {
		t.Fatal("did not expect UpdateJobStatus to be called")
	}
}

// TestAckJobStart_WrongStatus verifies 409 when the job is not in running state.
func TestAckJobStart_WrongStatus(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	nodeIDStr := nodeID.String()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusRunning,
		},
		getJobResult: store.Job{
			ID:     jobID.String(),
			RunID:  runID.String(),
			NodeID: &nodeIDStr,
			Name:   "mod-0",
			Status: store.JobStatusSucceeded, // Job already completed, not running
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobStatusCalled {
		t.Fatal("did not expect UpdateJobStatus to be called")
	}
}

// TestAckJobStart_JobNotFound verifies 404 when the job doesn't exist.
func TestAckJobStart_JobNotFound(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusQueued,
		},
		getJobErr: pgx.ErrNoRows, // Job not found
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobStatusCalled {
		t.Fatal("did not expect UpdateJobStatus to be called")
	}
}

// TestAckJobStart_JobRunMismatch verifies 400 when the job doesn't belong to the specified run.
func TestAckJobStart_JobRunMismatch(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	otherRunID := uuid.New()
	jobID := uuid.New()
	nodeIDStr := nodeID.String()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:     runID.String(),
			Status: store.RunStatusQueued,
		},
		getJobResult: store.Job{
			ID:     jobID.String(),
			RunID:  otherRunID.String(), // Different run
			NodeID: &nodeIDStr,
			Name:   "mod-0",
			Status: store.JobStatusRunning, // Jobs go directly to running on claim
		},
	}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobStatusCalled {
		t.Fatal("did not expect UpdateJobStatus to be called")
	}
}

// TestAckJobStart_MissingJobID verifies 400 when job_id is not provided.
func TestAckJobStart_MissingJobID(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{}

	handler := ackRunStartHandler(st, nil)
	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		// job_id omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestAckJobStart_PublishesEvent verifies that acknowledging a job publishes a running event.
func TestAckJobStart_PublishesEvent(t *testing.T) {
	t.Parallel()

	nodeID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	now := time.Now()
	nodeIDStr := nodeID.String()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID.String()},
		getRunResult: store.Run{
			ID:        runID.String(),
			NodeID:    &nodeIDStr,
			Status:    store.RunStatusQueued,
			RepoUrl:   "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getJobResult: store.Job{
			ID:     jobID.String(),
			RunID:  runID.String(),
			NodeID: &nodeIDStr,
			Name:   "pre-gate",
			Status: store.JobStatusRunning, // Jobs go directly to running on claim
		},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := ackRunStartHandler(st, eventsService)

	body, _ := json.Marshal(map[string]string{
		"run_id": runID.String(),
		"job_id": jobID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify a run event was published to the hub by checking the snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least one run event to be published")
	}

	// Verify the event type is "run".
	foundRunEvent := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRunEvent = true
			// Verify the event contains run state information with "running" status.
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
