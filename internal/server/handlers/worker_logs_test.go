package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// mustGzip returns the gzip-encoded bytes of data; fails the test on error.
func mustGzip(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func newNodeLogsHandler(t *testing.T, mockStore *mockStoreForLogs) http.HandlerFunc {
	t.Helper()
	eventsService, err := createTestEventsServiceWithStore(mockStore)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	bp := blobpersist.New(mockStore, bsmock.New())
	return createNodeLogsHandler(mockStore, bp, eventsService)
}

// TestCreateNodeLogsHandler_Success covers the happy path and asserts that
// the store recorded the create call.
func TestCreateNodeLogsHandler_Success(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{nodeExists: true}
	handler := newNodeLogsHandler(t, mockStore)

	nodeID := domaintypes.NewNodeKey()
	payload := map[string]any{
		"run_id":   domaintypes.NewRunID().String(),
		"job_id":   domaintypes.NewJobID().String(),
		"chunk_no": 0,
		"data":     mustGzip(t, []byte("test log line\n")),
	}
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/logs", payload, "id", nodeID)
	assertStatus(t, rr, http.StatusCreated)
	assertCalled(t, "CreateLog", mockStore.logCreated)
}

// TestCreateNodeLogsHandler_WithJobID verifies that job_id is propagated to the store.
func TestCreateNodeLogsHandler_WithJobID(t *testing.T) {
	t.Parallel()

	mockStore := &mockStoreForLogs{nodeExists: true}
	handler := newNodeLogsHandler(t, mockStore)

	nodeID := domaintypes.NewNodeKey()
	jobID := domaintypes.NewJobID()
	payload := map[string]any{
		"run_id":   domaintypes.NewRunID().String(),
		"job_id":   jobID.String(),
		"chunk_no": 1,
		"data":     mustGzip(t, []byte("hello with job id\n")),
	}
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/logs", payload, "id", nodeID)
	assertStatus(t, rr, http.StatusCreated)
	assertCalled(t, "CreateLog", mockStore.logCreated)
	if mockStore.lastCreateLog.JobID == nil {
		t.Fatal("expected JobID to be set")
	}
	if *mockStore.lastCreateLog.JobID != jobID {
		t.Fatalf("JobID mismatch: got %s, want %s", mockStore.lastCreateLog.JobID.String(), jobID.String())
	}
}

// TestCreateNodeLogsHandler_Errors covers the error/validation paths in one table.
func TestCreateNodeLogsHandler_Errors(t *testing.T) {
	t.Parallel()

	validRunID := domaintypes.NewRunID().String()
	tests := []struct {
		name       string
		nodeExists bool
		nodeID     string
		body       any
		wantStatus int
	}{
		{
			name:       "invalid_node_id",
			nodeExists: false,
			nodeID:     "invalid",
			body:       "{}",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "payload_too_large_decoded",
			nodeExists: true,
			nodeID:     domaintypes.NewNodeKey(),
			body: map[string]any{
				"run_id":   validRunID,
				"chunk_no": 0,
				"data":     make([]byte, 10<<20+1),
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "body_too_large",
			nodeExists: true,
			nodeID:     domaintypes.NewNodeKey(),
			body: map[string]any{
				"run_id":   validRunID,
				"chunk_no": 0,
				"data":     make([]byte, 16<<20),
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "missing_run_id",
			nodeExists: true,
			nodeID:     domaintypes.NewNodeKey(),
			body: map[string]any{
				"chunk_no": 0,
				"data":     []byte("test"),
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_data",
			nodeExists: true,
			nodeID:     domaintypes.NewNodeKey(),
			body: map[string]any{
				"run_id":   validRunID,
				"chunk_no": 0,
				"data":     []byte{},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "node_not_found",
			nodeExists: false,
			nodeID:     domaintypes.NewNodeKey(),
			body: map[string]any{
				"run_id":   validRunID,
				"chunk_no": 0,
				"data":     []byte("test"),
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStoreForLogs{nodeExists: tt.nodeExists}
			handler := newNodeLogsHandler(t, mockStore)

			rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+tt.nodeID+"/logs", tt.body, "id", tt.nodeID)
			assertStatus(t, rr, tt.wantStatus)
		})
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
