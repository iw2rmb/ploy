package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestHeartbeatHandler_BytesContract(t *testing.T) {
	st := &mockStore{}
	st.getNodeResult = store.Node{} // handler only checks err

	h := heartbeatHandler(st)
	nodeID := "aB3xY9"

	reqBody := `{
  "cpu_free_millis": 1500,
  "cpu_total_millis": 4000,
  "mem_free_bytes": 2147483648,
  "mem_total_bytes": 8589934592,
  "disk_free_bytes": 10737418240,
  "disk_total_bytes": 53687091200,
  "version": "ployd-node/test"
}`

	r := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/heartbeat", bytes.NewBufferString(reqBody))
	r.SetPathValue("id", nodeID)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusNoContent, w.Body.String())
	}

	if !st.getNodeCalled {
		t.Fatalf("expected GetNode to be called")
	}
	if !st.updateNodeHeartbeatCalled {
		t.Fatalf("expected UpdateNodeHeartbeat to be called")
	}

	if got := st.updateNodeHeartbeatParams.ID.String(); got != nodeID {
		t.Fatalf("UpdateNodeHeartbeat ID = %q, want %q", got, nodeID)
	}
	if st.updateNodeHeartbeatParams.CpuFreeMillis != 1500 {
		t.Fatalf("cpu_free_millis = %d, want %d", st.updateNodeHeartbeatParams.CpuFreeMillis, 1500)
	}
	if st.updateNodeHeartbeatParams.CpuTotalMillis != 4000 {
		t.Fatalf("cpu_total_millis = %d, want %d", st.updateNodeHeartbeatParams.CpuTotalMillis, 4000)
	}
	if st.updateNodeHeartbeatParams.MemFreeBytes != 2147483648 {
		t.Fatalf("mem_free_bytes = %d, want %d", st.updateNodeHeartbeatParams.MemFreeBytes, 2147483648)
	}
	if st.updateNodeHeartbeatParams.MemTotalBytes != 8589934592 {
		t.Fatalf("mem_total_bytes = %d, want %d", st.updateNodeHeartbeatParams.MemTotalBytes, 8589934592)
	}
	if st.updateNodeHeartbeatParams.DiskFreeBytes != 10737418240 {
		t.Fatalf("disk_free_bytes = %d, want %d", st.updateNodeHeartbeatParams.DiskFreeBytes, 10737418240)
	}
	if st.updateNodeHeartbeatParams.DiskTotalBytes != 53687091200 {
		t.Fatalf("disk_total_bytes = %d, want %d", st.updateNodeHeartbeatParams.DiskTotalBytes, 53687091200)
	}
	if st.updateNodeHeartbeatParams.Version != "ployd-node/test" {
		t.Fatalf("version = %q, want %q", st.updateNodeHeartbeatParams.Version, "ployd-node/test")
	}
}

func TestHeartbeatHandler_RejectsRedundantIdentity(t *testing.T) {
	st := &mockStore{}
	h := heartbeatHandler(st)
	nodeID := "aB3xY9"

	// node_id is redundant; the node identity is provided by the {id} path param.
	reqBody := `{
	  "node_id": "aB3xY9",
	  "cpu_free_millis": 1,
	  "cpu_total_millis": 1,
	  "mem_free_bytes": 1,
	  "mem_total_bytes": 1,
	  "disk_free_bytes": 1,
	  "disk_total_bytes": 1
	}`

	r := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/heartbeat", bytes.NewBufferString(reqBody))
	r.SetPathValue("id", nodeID)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if st.getNodeCalled || st.updateNodeHeartbeatCalled {
		t.Fatalf("expected no store calls on bad request (GetNode=%v UpdateNodeHeartbeat=%v)", st.getNodeCalled, st.updateNodeHeartbeatCalled)
	}
}

func TestHeartbeatHandler_RejectsLegacyMBFields(t *testing.T) {
	st := &mockStore{}
	h := heartbeatHandler(st)
	nodeID := "aB3xY9"

	reqBody := `{
  "cpu_free_millis": 1500,
  "cpu_total_millis": 4000,
  "mem_free_mb": 2048.0,
  "mem_total_mb": 8192.0,
  "disk_free_mb": 10240.0,
  "disk_total_mb": 51200.0
}`

	r := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/heartbeat", bytes.NewBufferString(reqBody))
	r.SetPathValue("id", nodeID)
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if st.getNodeCalled || st.updateNodeHeartbeatCalled {
		t.Fatalf("expected no store calls on bad request (GetNode=%v UpdateNodeHeartbeat=%v)", st.getNodeCalled, st.updateNodeHeartbeatCalled)
	}
}
