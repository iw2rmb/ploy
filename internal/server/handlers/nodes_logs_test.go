package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeLogs_Success(t *testing.T) {
	t.Parallel()
	st := &mockStore{}
	nodeID := uuid.New()
	runID := uuid.New()
	logID := int64(123)
	chunkNo := int32(5)

	// Mock GetNode to succeed (node exists).
	st.getNodeResult = store.Node{
		ID: nodeID.String(),
	}

	// Mock CreateLog to return a log with specified ID and ChunkNo.
	st.createLogResult = store.Log{
		ID:      logID,
		RunID:   runID.String(),
		ChunkNo: chunkNo,
		Data:    []byte{0x1f, 0x8b},
	}

	// Prepare request payload.
	reqBody := map[string]interface{}{
		"run_id":   runID.String(),
		"chunk_no": chunkNo,
		"data":     []byte{0x1f, 0x8b},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/logs", bytes.NewReader(bodyBytes))
	req.SetPathValue("id", nodeID.String())
	req.Header.Set("Content-Type", "application/json")

	createNodeLogsHandler(st, nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%s, want application/json", ct)
	}

	var resp nodeLogCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.ID != logID {
		t.Errorf("id=%d, want %d", resp.ID, logID)
	}
	if resp.ChunkNo != chunkNo {
		t.Errorf("chunk_no=%d, want %d", resp.ChunkNo, chunkNo)
	}
}
