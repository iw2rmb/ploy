package handlers

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

	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeLogsHandler_Success(t *testing.T) {
	t.Parallel()

	// Create mock store.
	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}

	handler := createNodeLogsHandler(mockStore, nil)

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
	jobID := uuid.New().String()
	payload := map[string]interface{}{
		"run_id":   runID,
		"job_id":   jobID,
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

func TestCreateNodeLogsHandler_WithBuildID(t *testing.T) {
	t.Parallel()

	// Create mock store.
	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}

	handler := createNodeLogsHandler(mockStore, nil)

	// Prepare gzipped test data.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte("hello with build id\n"))
	if err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	gzippedData := buf.Bytes()

	// Prepare request payload including build_id.
	runID := uuid.New().String()
	jobID := uuid.New().String()
	buildID := uuid.New().String()
	payload := map[string]interface{}{
		"run_id":   runID,
		"job_id":   jobID,
		"build_id": buildID,
		"chunk_no": 1,
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
		t.Fatalf("status code = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify build_id propagated to store.
	if !mockStore.logCreated {
		t.Fatal("log was not created in store")
	}
	if mockStore.lastCreateLog.BuildID == nil {
		t.Fatal("expected BuildID to be set")
	}
	if *mockStore.lastCreateLog.BuildID != buildID {
		t.Fatalf("BuildID mismatch: got %s, want %s", *mockStore.lastCreateLog.BuildID, buildID)
	}
}

func TestCreateNodeLogsHandler_InvalidNodeID(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{}
	handler := createNodeLogsHandler(mockStore, nil)

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
	handler := createNodeLogsHandler(mockStore, nil)

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
	handler := createNodeLogsHandler(mockStore, nil)

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
	handler := createNodeLogsHandler(mockStore, nil)

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
	handler := createNodeLogsHandler(mockStore, nil)

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
	handler := createNodeLogsHandler(mockStore, nil)

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
	nodeExists    bool
	logCreated    bool
	lastCreateLog store.CreateLogParams
}

func (m *mockStoreForLogs) GetNode(ctx context.Context, id string) (store.Node, error) {
	if !m.nodeExists {
		return store.Node{}, pgx.ErrNoRows
	}
	return store.Node{ID: id}, nil
}

func (m *mockStoreForLogs) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	m.logCreated = true
	m.lastCreateLog = arg
	return store.Log{
		ID:      1,
		RunID:   arg.RunID,
		JobID:   arg.JobID,
		BuildID: arg.BuildID,
		ChunkNo: arg.ChunkNo,
		Data:    arg.Data,
	}, nil
}
