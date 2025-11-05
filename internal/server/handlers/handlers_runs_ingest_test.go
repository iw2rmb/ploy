package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// ----- run logs -----
type mockStoreRunLogs struct {
	store.Store
	lastCreate store.CreateLogParams
}

func (m *mockStoreRunLogs) CreateLog(_ context.Context, arg store.CreateLogParams) (store.Log, error) {
	m.lastCreate = arg
	return store.Log{ID: 1, RunID: arg.RunID, StageID: arg.StageID, BuildID: arg.BuildID, ChunkNo: arg.ChunkNo, Data: arg.Data}, nil
}

func TestCreateRunLogsHandler_Success(t *testing.T) {
	ms := &mockStoreRunLogs{}
	h := createRunLogHandler(ms, nil)
	runID := uuid.New().String()
	stageID := uuid.New().String()
	buildID := uuid.New().String()
	payload := map[string]any{"stage_id": stageID, "build_id": buildID, "chunk_no": 2, "data": []byte("hello")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if uuid.UUID(ms.lastCreate.RunID.Bytes).String() != runID {
		t.Fatalf("runID mismatch")
	}
}

func TestCreateRunLogsHandler_TooLarge(t *testing.T) {
	ms := &mockStoreRunLogs{}
	h := createRunLogHandler(ms, nil)
	runID := uuid.New().String()
	big := make([]byte, 1<<20+1)
	payload := map[string]any{"chunk_no": 0, "data": big}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/logs", bytes.NewReader(b))
	req.SetPathValue("id", runID)
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
	stage   store.Stage
	run     store.Run
	created store.CreateDiffParams
}

func (m *mockStoreRunDiffs) GetStage(_ context.Context, id pgtype.UUID) (store.Stage, error) {
	return m.stage, nil
}
func (m *mockStoreRunDiffs) GetRun(_ context.Context, id pgtype.UUID) (store.Run, error) {
	return m.run, nil
}
func (m *mockStoreRunDiffs) CreateDiff(_ context.Context, p store.CreateDiffParams) (store.Diff, error) {
	m.created = p
	return store.Diff{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, nil
}

func TestCreateRunDiffHandler_Success(t *testing.T) {
	runID := uuid.New()
	stageID := uuid.New()
	ms := &mockStoreRunDiffs{
		stage: store.Stage{ID: pgtype.UUID{Bytes: stageID, Valid: true}, RunID: pgtype.UUID{Bytes: runID, Valid: true}},
		run:   store.Run{ID: pgtype.UUID{Bytes: runID, Valid: true}},
	}
	h := createRunDiffHandler(ms)
	payload := map[string]any{"stage_id": stageID.String(), "patch": []byte("gz-diff"), "summary": map[string]any{"k": "v"}}
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
	stage   store.Stage
	run     store.Run
	created store.CreateArtifactBundleParams
}

func (m *mockStoreRunArtifacts) GetStage(_ context.Context, id pgtype.UUID) (store.Stage, error) {
	return m.stage, nil
}
func (m *mockStoreRunArtifacts) GetRun(_ context.Context, id pgtype.UUID) (store.Run, error) {
	return m.run, nil
}
func (m *mockStoreRunArtifacts) CreateArtifactBundle(_ context.Context, p store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	m.created = p
	return store.ArtifactBundle{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, nil
}

func TestCreateRunArtifactBundleHandler_Success(t *testing.T) {
	runID := uuid.New()
	stageID := uuid.New()
	ms := &mockStoreRunArtifacts{
		stage: store.Stage{ID: pgtype.UUID{Bytes: stageID, Valid: true}, RunID: pgtype.UUID{Bytes: runID, Valid: true}},
		run:   store.Run{ID: pgtype.UUID{Bytes: runID, Valid: true}},
	}
	h := createRunArtifactBundleHandler(ms)
	payload := map[string]any{"stage_id": stageID.String(), "bundle": []byte("gz-tar")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/artifact_bundles", bytes.NewReader(b))
	req.SetPathValue("id", runID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateModArtifactBundleHandler_Success(t *testing.T) {
	modID := uuid.New()
	stageID := uuid.New()
	ms := &mockStoreRunArtifacts{
		stage: store.Stage{ID: pgtype.UUID{Bytes: stageID, Valid: true}, RunID: pgtype.UUID{Bytes: modID, Valid: true}},
		run:   store.Run{ID: pgtype.UUID{Bytes: modID, Valid: true}},
	}
	h := createRunArtifactBundleHandler(ms)
	payload := map[string]any{"stage_id": stageID.String(), "bundle": []byte("gz-tar")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/artifact_bundles", bytes.NewReader(b))
	req.SetPathValue("id", modID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateModArtifactBundleHandler_TooLarge(t *testing.T) {
	modID := uuid.New()
	ms := &mockStoreRunArtifacts{
		run: store.Run{ID: pgtype.UUID{Bytes: modID, Valid: true}},
	}
	h := createRunArtifactBundleHandler(ms)
	big := make([]byte, 1<<20+1)
	payload := map[string]any{"bundle": big}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/artifact_bundles", bytes.NewReader(b))
	req.SetPathValue("id", modID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 got %d", rr.Code)
	}
}

// Ensure 404 is returned when the ticket (run) does not exist.
type mockStoreRunArtifactsNotFound struct {
	store.Store
}

func (m *mockStoreRunArtifactsNotFound) GetRun(_ context.Context, id pgtype.UUID) (store.Run, error) {
	return store.Run{}, pgx.ErrNoRows
}

func TestCreateModArtifactBundleHandler_RunNotFound(t *testing.T) {
	modID := uuid.New()
	ms := &mockStoreRunArtifactsNotFound{}
	h := createRunArtifactBundleHandler(ms)
	payload := map[string]any{"bundle": []byte("gz-tar")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+modID.String()+"/artifact_bundles", bytes.NewReader(b))
	req.SetPathValue("id", modID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", rr.Code, rr.Body.String())
	}
}

// legacy jobs tests removed with legacy endpoints.
