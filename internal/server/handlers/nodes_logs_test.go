package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeLogs_Success(t *testing.T) {
	t.Parallel()
	st := &nodeStore{}
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	logID := int64(123)
	chunkNo := int32(5)

	// Mock GetNode to succeed (node exists).
	st.getNode.val = store.Node{
		ID: nodeID,
	}

	// Mock CreateLog to return a log with specified ID and ChunkNo.
	objKey := fmt.Sprintf("logs/run/%s/job/none/chunk/%d/log/%d.gz", runID.String(), chunkNo, logID)
	st.createLog.val = store.Log{
		ID:        logID,
		RunID:     runID,
		ChunkNo:   chunkNo,
		DataSize:  2,
		ObjectKey: &objKey,
	}

	// Create events service with the mock store — required for log ingestion.
	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	// Create blobpersist service for coordinated writes.
	bp := blobpersist.New(st, bsmock.New())

	// Prepare request payload.
	reqBody := map[string]interface{}{
		"run_id":   runID.String(),
		"chunk_no": chunkNo,
		"data":     []byte{0x1f, 0x8b},
	}

	handler := createNodeLogsHandler(st, bp, eventsService)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/logs", reqBody, "id", nodeIDStr)

	assertStatus(t, rr, http.StatusCreated)

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
