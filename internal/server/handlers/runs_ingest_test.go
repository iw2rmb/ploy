package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// ----- run logs -----
type mockStoreRunLogs struct {
	store.Store
	lastCreate store.CreateLogParams
}

// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
func (m *mockStoreRunLogs) CreateLog(_ context.Context, arg store.CreateLogParams) (store.Log, error) {
	m.lastCreate = arg
	jobKey := "none"
	if arg.JobID != nil && !arg.JobID.IsZero() {
		jobKey = arg.JobID.String()
	}
	objKey := "logs/run/" + arg.RunID.String() + "/job/" + jobKey + "/chunk/" + fmt.Sprintf("%d", arg.ChunkNo) + "/log/1.gz"
	return store.Log{ID: 1, RunID: arg.RunID, JobID: arg.JobID, ChunkNo: arg.ChunkNo, DataSize: arg.DataSize, ObjectKey: &objKey}, nil
}

// GetJob returns an empty job for log enrichment (no-op for this test).
func (m *mockStoreRunLogs) GetJob(_ context.Context, id domaintypes.JobID) (store.Job, error) {
	return store.Job{}, nil
}

func TestCreateRunLogsHandler_Success(t *testing.T) {
	ms := &mockStoreRunLogs{}
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(ms)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(ms, bsmock.New())
	h := createRunLogHandler(ms, bp, eventsService)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	// Note: build_id removed; logs are now grouped at job level only.
	payload := map[string]any{"job_id": jobIDStr, "chunk_no": 2, "data": []byte("hello")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ms.lastCreate.RunID != runID {
		t.Fatalf("runID mismatch")
	}
}

func TestCreateRunLogsHandler_TooLarge(t *testing.T) {
	ms := &mockStoreRunLogs{}
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(ms)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(ms, bsmock.New())
	h := createRunLogHandler(ms, bp, eventsService)
	runID := domaintypes.NewRunID()
	big := make([]byte, 10<<20+1)
	payload := map[string]any{"chunk_no": 0, "data": big}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 got %d", rr.Code)
	}
}

// ----- run diffs -----
type mockStoreRunDiffs struct {
	store.Store
	job     store.Job
	run     store.Run
	created store.CreateDiffParams
}

func (m *mockStoreRunDiffs) GetJob(_ context.Context, id domaintypes.JobID) (store.Job, error) {
	return m.job, nil
}
func (m *mockStoreRunDiffs) GetRun(_ context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.run, nil
}
func (m *mockStoreRunDiffs) CreateDiff(_ context.Context, p store.CreateDiffParams) (store.Diff, error) {
	m.created = p
	objKey := "diffs/run/" + p.RunID.String() + "/diff/" + uuid.New().String() + ".patch.gz"
	return store.Diff{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, PatchSize: p.PatchSize, ObjectKey: &objKey}, nil
}

func TestCreateRunDiffHandler_Success(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	ms := &mockStoreRunDiffs{
		job: store.Job{ID: jobID, RunID: runID},
		run: store.Run{ID: runID},
	}
	bp := blobpersist.New(ms, bsmock.New())
	h := createRunDiffHandler(ms, bp)
	payload := map[string]any{"job_id": jobID.String(), "patch": []byte("gz-diff"), "summary": map[string]any{"k": "v"}}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/diffs", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// ----- run artifacts -----
type mockStoreRunArtifacts struct {
	store.Store
	job     store.Job
	run     store.Run
	created store.CreateArtifactBundleParams
}

func (m *mockStoreRunArtifacts) GetJob(_ context.Context, id domaintypes.JobID) (store.Job, error) {
	return m.job, nil
}
func (m *mockStoreRunArtifacts) GetRun(_ context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.run, nil
}
func (m *mockStoreRunArtifacts) CreateArtifactBundle(_ context.Context, p store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	m.created = p
	cid := "bafy-test"
	digest := "sha256:test"
	objKey := "artifacts/run/" + p.RunID.String() + "/bundle/" + uuid.New().String() + ".tar.gz"
	return store.ArtifactBundle{
		ID:         pgtype.UUID{Bytes: uuid.New(), Valid: true},
		RunID:      p.RunID,
		JobID:      p.JobID,
		Name:       p.Name,
		BundleSize: p.BundleSize,
		ObjectKey:  &objKey,
		Cid:        &cid,
		Digest:     &digest,
	}, nil
}

func TestCreateRunArtifactBundleHandler_Success(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	ms := &mockStoreRunArtifacts{
		job: store.Job{ID: jobID, RunID: runID, NodeID: &nodeID},
		run: store.Run{ID: runID},
	}
	bp := blobpersist.New(ms, bsmock.New())
	h := createJobArtifactHandler(ms, bp)
	payload := map[string]any{"name": "artifact-name", "bundle": []byte("gz-tar")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/jobs/"+jobID.String()+"/artifact", bytes.NewReader(b))
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nodeUUIDHeader, nodeIDStr)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateMigArtifactBundleHandler_Success(t *testing.T) {
	t.Skip("mod-scoped artifact upload endpoint removed; use job-scoped /v1/runs/{run_id}/jobs/{job_id}/artifact")
}

func TestCreateMigArtifactBundleHandler_TooLarge(t *testing.T) {
	t.Skip("mod-scoped artifact upload endpoint removed; use job-scoped /v1/runs/{run_id}/jobs/{job_id}/artifact")
}

func TestCreateMigArtifactBundleHandler_RunNotFound(t *testing.T) {
	t.Skip("mod-scoped artifact upload endpoint removed; use job-scoped /v1/runs/{run_id}/jobs/{job_id}/artifact")
}

// legacy jobs tests removed with legacy endpoints.
