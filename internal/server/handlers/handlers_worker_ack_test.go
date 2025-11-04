package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestAckRunStart_Success verifies 204 and store transition when the run
// is assigned to the requesting node.
func TestAckRunStart_Success(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st)

	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.ackRunStartCalled {
		t.Fatal("expected AckRunStart to be called")
	}
	if st.ackRunStartParam.Bytes != runID {
		t.Fatalf("AckRunStart called with wrong run id: %v", st.ackRunStartParam)
	}
}

// TestAckRunStart_WrongNode verifies 403 when the run is assigned to a different node.
func TestAckRunStart_WrongNode(t *testing.T) {
	nodeID := uuid.New()
	otherNode := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: otherNode, Valid: true},
			Status: store.RunStatusAssigned,
		},
	}

	handler := ackRunStartHandler(st)
	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if st.ackRunStartCalled {
		t.Fatal("did not expect AckRunStart to be called")
	}
}

// TestAckRunStart_WrongStatus verifies 409 when the run is not in assigned state.
func TestAckRunStart_WrongStatus(t *testing.T) {
	nodeID := uuid.New()
	runID := uuid.New()

	st := &mockStore{
		getNodeResult: store.Node{ID: pgtype.UUID{Bytes: nodeID, Valid: true}},
		getRunResult: store.Run{
			ID:     pgtype.UUID{Bytes: runID, Valid: true},
			NodeID: pgtype.UUID{Bytes: nodeID, Valid: true},
			Status: store.RunStatusRunning, // not assigned
		},
	}

	handler := ackRunStartHandler(st)
	body, _ := json.Marshal(map[string]string{"run_id": runID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/ack", bytes.NewReader(body))
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if st.ackRunStartCalled {
		t.Fatal("did not expect AckRunStart to be called")
	}
}
