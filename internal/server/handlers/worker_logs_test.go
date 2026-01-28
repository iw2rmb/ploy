package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeLogsHandler_Success(t *testing.T) {
	t.Parallel()

	// Create mock store.
	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}

	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Prepare gzipped test data.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err = gzWriter.Write([]byte("test log line\n")); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err = gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	gzippedData := buf.Bytes()

	// Prepare request payload.
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	payload := map[string]interface{}{
		"run_id":   runID.String(),
		"job_id":   jobID.String(),
		"chunk_no": 0,
		"data":     gzippedData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// Create request.
	nodeID := domaintypes.NewNodeKey()
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

// TestCreateNodeLogsHandler_WithJobID verifies that job_id is propagated to the store.
// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
func TestCreateNodeLogsHandler_WithJobID(t *testing.T) {
	t.Parallel()

	// Create mock store.
	mockStore := &mockStoreForLogs{
		nodeExists: true,
	}

	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Prepare gzipped test data.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err = gzWriter.Write([]byte("hello with job id\n")); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err = gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	gzippedData := buf.Bytes()

	// Prepare request payload including job_id.
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	payload := map[string]interface{}{
		"run_id":   runID.String(),
		"job_id":   jobID.String(),
		"chunk_no": 1,
		"data":     gzippedData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// Create request.
	nodeID := domaintypes.NewNodeKey()
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

	// Verify job_id propagated to store.
	if !mockStore.logCreated {
		t.Fatal("log was not created in store")
	}
	if mockStore.lastCreateLog.JobID == nil {
		t.Fatal("expected JobID to be set")
	}
	if *mockStore.lastCreateLog.JobID != jobID {
		t.Fatalf("JobID mismatch: got %s, want %s", mockStore.lastCreateLog.JobID.String(), jobID.String())
	}
}

func TestCreateNodeLogsHandler_InvalidNodeID(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{}
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

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
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Create decoded payload larger than 10 MiB (will trigger 413 after decode).
	largeData := make([]byte, 10<<20+1)
	payload := map[string]interface{}{
		"run_id":   domaintypes.NewRunID().String(),
		"chunk_no": 0,
		"data":     largeData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := domaintypes.NewNodeKey()
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
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Craft a request whose JSON body exceeds 16 MiB due to base64 overhead.
	// Any payload large enough to trip the body cap will also exceed the decoded cap,
	// but MaxBytesReader should fail early before decode.
	hugeData := make([]byte, 16<<20)
	payload := map[string]interface{}{
		"run_id":   domaintypes.NewRunID().String(),
		"chunk_no": 0,
		"data":     hugeData,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := domaintypes.NewNodeKey()
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
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Create payload without run_id.
	payload := map[string]interface{}{
		"chunk_no": 0,
		"data":     []byte("test"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := domaintypes.NewNodeKey()
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
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Create payload with empty data.
	payload := map[string]interface{}{
		"run_id":   domaintypes.NewRunID().String(),
		"chunk_no": 0,
		"data":     []byte{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := domaintypes.NewNodeKey()
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
	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	handler := createNodeLogsHandler(mockStore, bp, eventsService)

	// Create valid payload.
	payload := map[string]interface{}{
		"run_id":   domaintypes.NewRunID().String(),
		"chunk_no": 0,
		"data":     []byte("test"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	nodeID := domaintypes.NewNodeKey()
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

func (m *mockStoreForLogs) GetNode(ctx context.Context, id domaintypes.NodeID) (store.Node, error) {
	if !m.nodeExists {
		return store.Node{}, pgx.ErrNoRows
	}
	return store.Node{ID: id}, nil
}

func (m *mockStoreForLogs) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	m.logCreated = true
	m.lastCreateLog = arg
	// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
	jobKey := "none"
	if arg.JobID != nil && !arg.JobID.IsZero() {
		jobKey = arg.JobID.String()
	}
	objKey := fmt.Sprintf("logs/run/%s/job/%s/chunk/%d/log/1.gz", arg.RunID.String(), jobKey, arg.ChunkNo)
	return store.Log{
		ID:        1,
		RunID:     arg.RunID,
		JobID:     arg.JobID,
		ChunkNo:   arg.ChunkNo,
		DataSize:  arg.DataSize,
		ObjectKey: &objKey,
	}, nil
}

// GetJob returns an empty job for log enrichment (no-op for this test).
func (m *mockStoreForLogs) GetJob(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
	return store.Job{}, nil
}
