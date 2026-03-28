package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateRunLogsHandler_Success(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "logs/run/" + runID.String() + "/log/1.gz"
	st := &mockStore{
		createLogResult: store.Log{ID: 1, RunID: runID, JobID: &jobID, ChunkNo: 2, DataSize: 5, ObjectKey: &objKey},
	}
	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createRunLogHandler(st, bp, eventsService)
	payload := map[string]any{"job_id": jobID.String(), "chunk_no": 2, "data": []byte("hello")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusCreated)
}

func TestCreateRunLogsHandler_TooLarge(t *testing.T) {
	st := &mockStore{}
	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createRunLogHandler(st, bp, eventsService)
	runID := domaintypes.NewRunID()
	big := make([]byte, 10<<20+1)
	payload := map[string]any{"chunk_no": 0, "data": big}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusRequestEntityTooLarge)
}

func TestCreateRunDiffHandler_Success(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "diffs/run/" + runID.String() + "/diff/1.patch.gz"
	st := &mockStore{
		getJobResult:    store.Job{ID: jobID, RunID: runID},
		getRunResult:    store.Run{ID: runID},
		createDiffResult: store.Diff{ID: pgtype.UUID{Valid: true}, PatchSize: 7, ObjectKey: &objKey},
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createRunDiffHandler(st, bp)
	payload := map[string]any{"job_id": jobID.String(), "patch": []byte("gz-diff"), "summary": map[string]any{"k": "v"}}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/diffs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusCreated)
}

func TestCreateRunArtifactBundleHandler_Success(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	objKey := "artifacts/run/" + runID.String() + "/bundle/1.tar.gz"
	cid := "bafy-test"
	digest := "sha256:test"
	st := &mockStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, NodeID: &nodeID},
		getRunResult: store.Run{ID: runID},
		createArtifactBundleResult: store.ArtifactBundle{
			ID:        pgtype.UUID{Valid: true},
			RunID:     runID,
			JobID:     &jobID,
			Name:      strPtr("artifact-name"),
			ObjectKey: &objKey,
			Cid:       &cid,
			Digest:    &digest,
		},
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createJobArtifactHandler(st, bp)
	payload := map[string]any{"name": "artifact-name", "bundle": []byte("gz-tar")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/jobs/"+jobID.String()+"/artifact", bytes.NewReader(b))
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusCreated)
}
