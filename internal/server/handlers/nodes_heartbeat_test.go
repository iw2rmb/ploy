package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestHeartbeatHandler_BytesContract(t *testing.T) {
	st := &nodeStore{}
	st.getNode.val = store.Node{} // handler only checks err

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

	rr := doRequest(t, h, http.MethodPost, "/v1/nodes/"+nodeID+"/heartbeat", reqBody, "id", nodeID)

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "GetNode", st.getNode.called)
	assertCalled(t, "UpdateNodeHeartbeat", st.updateNodeHeartbeat.called)

	// Contract: verify all 8 fields are forwarded to the store.
	if got := st.updateNodeHeartbeat.params.ID.String(); got != nodeID {
		t.Fatalf("UpdateNodeHeartbeat ID = %q, want %q", got, nodeID)
	}
	if st.updateNodeHeartbeat.params.CpuFreeMillis != 1500 {
		t.Fatalf("cpu_free_millis = %d, want %d", st.updateNodeHeartbeat.params.CpuFreeMillis, 1500)
	}
	if st.updateNodeHeartbeat.params.CpuTotalMillis != 4000 {
		t.Fatalf("cpu_total_millis = %d, want %d", st.updateNodeHeartbeat.params.CpuTotalMillis, 4000)
	}
	if st.updateNodeHeartbeat.params.MemFreeBytes != 2147483648 {
		t.Fatalf("mem_free_bytes = %d, want %d", st.updateNodeHeartbeat.params.MemFreeBytes, 2147483648)
	}
	if st.updateNodeHeartbeat.params.MemTotalBytes != 8589934592 {
		t.Fatalf("mem_total_bytes = %d, want %d", st.updateNodeHeartbeat.params.MemTotalBytes, 8589934592)
	}
	if st.updateNodeHeartbeat.params.DiskFreeBytes != 10737418240 {
		t.Fatalf("disk_free_bytes = %d, want %d", st.updateNodeHeartbeat.params.DiskFreeBytes, 10737418240)
	}
	if st.updateNodeHeartbeat.params.DiskTotalBytes != 53687091200 {
		t.Fatalf("disk_total_bytes = %d, want %d", st.updateNodeHeartbeat.params.DiskTotalBytes, 53687091200)
	}
	if st.updateNodeHeartbeat.params.Version != "ployd-node/test" {
		t.Fatalf("version = %q, want %q", st.updateNodeHeartbeat.params.Version, "ployd-node/test")
	}
}

func TestHeartbeatHandler_RejectsInvalidBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "redundant_identity",
			body: `{
  "node_id": "aB3xY9",
  "cpu_free_millis": 1,
  "cpu_total_millis": 1,
  "mem_free_bytes": 1,
  "mem_total_bytes": 1,
  "disk_free_bytes": 1,
  "disk_total_bytes": 1
}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &nodeStore{}
			h := heartbeatHandler(st)
			nodeID := "aB3xY9"

			rr := doRequest(t, h, http.MethodPost, "/v1/nodes/"+nodeID+"/heartbeat", tc.body, "id", nodeID)

			assertStatus(t, rr, http.StatusBadRequest)
			assertNotCalled(t, "GetNode", st.getNode.called)
			assertNotCalled(t, "UpdateNodeHeartbeat", st.updateNodeHeartbeat.called)
		})
	}
}
