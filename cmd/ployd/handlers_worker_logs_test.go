package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeLogsHandler_Success(t *testing.T) {
	t.Parallel()

	// Create mock store.
	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}

	handler := createNodeLogsHandler(mockStore)

	// Prepare gzipped test data.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte("test log line\n"))
	if err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	gzippedData := buf.Bytes()

	// Prepare request payload.
	runID := uuid.New().String()
	stageID := uuid.New().String()
	payload := map[string]interface{}{
		"run_id":   runID,
		"stage_id": stageID,
		"chunk_no": 0,
		"data":     gzippedData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// Create request.
	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder.
	w := httptest.NewRecorder()

	// Call handler.
	handler(w, req)

	// Check response.
	if w.Code != http.StatusCreated {
		t.Errorf("status code = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify log was created.
	if !mockStore.logCreated {
		t.Error("log was not created in store")
	}
}

func TestCreateNodeLogsHandler_InvalidNodeID(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{}
	handler := createNodeLogsHandler(mockStore)

	// Create request with invalid node ID.
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/invalid/logs", strings.NewReader("{}"))
	req.SetPathValue("id", "invalid")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateNodeLogsHandler_PayloadTooLarge(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}
	handler := createNodeLogsHandler(mockStore)

	// Create decoded payload larger than 1 MiB (will trigger 413 after decode).
	largeData := make([]byte, 1<<20+1)
	payload := map[string]interface{}{
		"run_id":   uuid.New().String(),
		"chunk_no": 0,
		"data":     largeData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCreateNodeLogsHandler_BodyTooLarge(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}
	handler := createNodeLogsHandler(mockStore)

	// Craft a request whose JSON body exceeds 2 MiB due to base64 overhead.
	// 2 MiB raw → ~2.66 MiB base64 → trips MaxBytesReader body cap.
	hugeData := make([]byte, 2<<20)
	payload := map[string]interface{}{
		"run_id":   uuid.New().String(),
		"chunk_no": 0,
		"data":     hugeData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCreateNodeLogsHandler_MissingRunID(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}
	handler := createNodeLogsHandler(mockStore)

	// Create payload without run_id.
	payload := map[string]interface{}{
		"chunk_no": 0,
		"data":     []byte("test"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateNodeLogsHandler_EmptyData(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}
	handler := createNodeLogsHandler(mockStore)

	// Create payload with empty data.
	payload := map[string]interface{}{
		"run_id":   uuid.New().String(),
		"chunk_no": 0,
		"data":     []byte{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateNodeLogsHandler_NodeNotFound(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{
		nodeExists: false,
	}
	handler := createNodeLogsHandler(mockStore)

	// Create valid payload.
	payload := map[string]interface{}{
		"run_id":   uuid.New().String(),
		"chunk_no": 0,
		"data":     []byte("test"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/logs", bytes.NewReader(body))
	req.SetPathValue("id", nodeID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// mockStoreForLogs is a minimal mock store for testing log handlers.
type mockStoreForLogs struct {
	store.Store
	nodeExists bool
	logCreated bool
}

func (m *mockStoreForLogs) GetNode(ctx context.Context, id pgtype.UUID) (store.Node, error) {
	if !m.nodeExists {
		return store.Node{}, pgx.ErrNoRows
	}
	return store.Node{ID: id}, nil
}

func (m *mockStoreForLogs) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	m.logCreated = true
	return store.Log{
		ID:      1,
		RunID:   arg.RunID,
		StageID: arg.StageID,
		BuildID: arg.BuildID,
		ChunkNo: arg.ChunkNo,
		Data:    arg.Data,
	}, nil
}
