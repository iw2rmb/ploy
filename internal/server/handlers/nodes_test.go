package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestDrainNodeHandlerSuccess verifies successful node draining.
func TestDrainNodeHandlerSuccess(t *testing.T) {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{
			ID:            nodeID,
			Name:          "worker-1",
			IpAddress:     netip.MustParseAddr("10.0.0.1"),
			Concurrency:   4,
			Drained:       false,
			CreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
			LastHeartbeat: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := drainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/drain", nil, "id", nodeIDStr)

	assertStatus(t, rr, http.StatusNoContent)

	if !st.getNodeCalled {
		t.Error("expected GetNode to be called")
	}
	if !st.updateNodeDrainedCalled {
		t.Error("expected UpdateNodeDrained to be called")
	}
	if !st.updateNodeDrainedParams.Drained {
		t.Error("expected drained flag to be true")
	}
}

// TestDrainNodeHandlerInvalidID verifies rejection of invalid node IDs.
// Node IDs are NanoID(6) strings; invalid IDs are rejected.
func TestDrainNodeHandlerInvalidID(t *testing.T) {
	st := &mockStore{}
	handler := drainNodeHandler(st)

	cases := []struct {
		name  string
		id    string
		urlID string
	}{
		{"empty id", "", "x"},
		{"whitespace", "   ", "x"},
		{"invalid nanoid", "not-a-nanoid", "x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+tc.urlID+"/drain", nil, "id", tc.id)

			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

// TestDrainNodeHandlerNotFound verifies 404 when node doesn't exist.
func TestDrainNodeHandlerNotFound(t *testing.T) {
	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{
		getNodeErr: pgx.ErrNoRows,
	}

	handler := drainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/drain", nil, "id", nodeID)

	assertStatus(t, rr, http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "node not found") {
		t.Errorf("expected error about node not found, got: %s", rr.Body.String())
	}
}

// TestDrainNodeHandlerAlreadyDrained verifies 409 when node is already drained.
func TestDrainNodeHandlerAlreadyDrained(t *testing.T) {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{
			ID:            nodeID,
			Name:          "worker-1",
			IpAddress:     netip.MustParseAddr("10.0.0.1"),
			Concurrency:   4,
			Drained:       true, // Already drained
			CreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
			LastHeartbeat: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := drainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/drain", nil, "id", nodeIDStr)

	assertStatus(t, rr, http.StatusConflict)
	if !strings.Contains(rr.Body.String(), "already drained") {
		t.Errorf("expected error about already drained, got: %s", rr.Body.String())
	}
	if st.updateNodeDrainedCalled {
		t.Error("expected UpdateNodeDrained not to be called")
	}
}

// TestUndrainNodeHandlerSuccess verifies successful node undraining.
func TestUndrainNodeHandlerSuccess(t *testing.T) {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{
			ID:            nodeID,
			Name:          "worker-1",
			IpAddress:     netip.MustParseAddr("10.0.0.1"),
			Concurrency:   4,
			Drained:       true, // Currently drained
			CreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
			LastHeartbeat: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := undrainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/undrain", nil, "id", nodeIDStr)

	assertStatus(t, rr, http.StatusNoContent)

	if !st.getNodeCalled {
		t.Error("expected GetNode to be called")
	}
	if !st.updateNodeDrainedCalled {
		t.Error("expected UpdateNodeDrained to be called")
	}
	if st.updateNodeDrainedParams.Drained {
		t.Error("expected drained flag to be false")
	}
}

// TestUndrainNodeHandlerInvalidID verifies rejection of invalid node IDs.
// Node IDs are NanoID(6) strings; invalid IDs are rejected.
func TestUndrainNodeHandlerInvalidID(t *testing.T) {
	st := &mockStore{}
	handler := undrainNodeHandler(st)

	cases := []struct {
		name  string
		id    string
		urlID string
	}{
		{"empty id", "", "x"},
		{"whitespace", "   ", "x"},
		{"invalid nanoid", "not-a-nanoid", "x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+tc.urlID+"/undrain", nil, "id", tc.id)

			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

// TestUndrainNodeHandlerNotFound verifies 404 when node doesn't exist.
func TestUndrainNodeHandlerNotFound(t *testing.T) {
	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{
		getNodeErr: pgx.ErrNoRows,
	}

	handler := undrainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/undrain", nil, "id", nodeID)

	assertStatus(t, rr, http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "node not found") {
		t.Errorf("expected error about node not found, got: %s", rr.Body.String())
	}
}

// TestUndrainNodeHandlerNotDrained verifies 409 when node is not drained.
func TestUndrainNodeHandlerNotDrained(t *testing.T) {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	now := time.Now()

	st := &mockStore{
		getNodeResult: store.Node{
			ID:            nodeID,
			Name:          "worker-1",
			IpAddress:     netip.MustParseAddr("10.0.0.1"),
			Concurrency:   4,
			Drained:       false, // Not drained
			CreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
			LastHeartbeat: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := undrainNodeHandler(st)
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/undrain", nil, "id", nodeIDStr)

	assertStatus(t, rr, http.StatusConflict)
	if !strings.Contains(rr.Body.String(), "not drained") {
		t.Errorf("expected error about not drained, got: %s", rr.Body.String())
	}
	if st.updateNodeDrainedCalled {
		t.Error("expected UpdateNodeDrained not to be called")
	}
}

// TestListNodesHandlerSuccess verifies successful node listing.
func TestListNodesHandlerSuccess(t *testing.T) {
	node1ID := domaintypes.NodeID(domaintypes.NewNodeKey())
	node2ID := domaintypes.NodeID(domaintypes.NewNodeKey())
	now := time.Now()

	st := &mockStore{
		listNodesResult: []store.Node{
			{
				ID:              node1ID,
				Name:            "worker-1",
				IpAddress:       netip.MustParseAddr("10.0.0.1"),
				Version:         strPtr("v1.0.0"),
				Concurrency:     4,
				CpuTotalMillis:  4000,
				CpuFreeMillis:   2000,
				MemTotalBytes:   8589934592,
				MemFreeBytes:    4294967296,
				DiskTotalBytes:  107374182400,
				DiskFreeBytes:   53687091200,
				CertSerial:      strPtr("123456"),
				CertFingerprint: strPtr("aa:bb:cc"),
				CertNotBefore:   pgtype.Timestamptz{Time: now, Valid: true},
				CertNotAfter:    pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true},
				LastHeartbeat:   pgtype.Timestamptz{Time: now, Valid: true},
				Drained:         false,
				CreatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				ID:             node2ID,
				Name:           "worker-2",
				IpAddress:      netip.MustParseAddr("10.0.0.2"),
				Concurrency:    2,
				CpuTotalMillis: 2000,
				CpuFreeMillis:  1000,
				MemTotalBytes:  4294967296,
				MemFreeBytes:   2147483648,
				DiskTotalBytes: 53687091200,
				DiskFreeBytes:  26843545600,
				Drained:        true,
				CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
	}

	handler := listNodesHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	var resp []struct {
		ID              string  `json:"id"`
		Name            string  `json:"name"`
		IPAddress       string  `json:"ip_address"`
		Version         *string `json:"version,omitempty"`
		Concurrency     int32   `json:"concurrency"`
		CPUTotalMillis  int32   `json:"cpu_total_millis"`
		CPUFreeMillis   int32   `json:"cpu_free_millis"`
		MemTotalBytes   int64   `json:"mem_total_bytes"`
		MemFreeBytes    int64   `json:"mem_free_bytes"`
		DiskTotalBytes  int64   `json:"disk_total_bytes"`
		DiskFreeBytes   int64   `json:"disk_free_bytes"`
		CertSerial      *string `json:"cert_serial,omitempty"`
		CertFingerprint *string `json:"cert_fingerprint,omitempty"`
		CertNotBefore   *string `json:"cert_not_before,omitempty"`
		CertNotAfter    *string `json:"cert_not_after,omitempty"`
		LastHeartbeat   *string `json:"last_heartbeat,omitempty"`
		Drained         bool    `json:"drained"`
		CreatedAt       string  `json:"created_at"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(resp))
	}

	// Check first node.
	if resp[0].ID != node1ID.String() {
		t.Errorf("expected id %s, got %s", node1ID.String(), resp[0].ID)
	}
	if resp[0].Name != "worker-1" {
		t.Errorf("expected name worker-1, got %s", resp[0].Name)
	}
	if resp[0].IPAddress != "10.0.0.1" {
		t.Errorf("expected ip_address 10.0.0.1, got %s", resp[0].IPAddress)
	}
	if resp[0].Drained {
		t.Error("expected first node not to be drained")
	}
	if resp[0].Version == nil || *resp[0].Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0")
	}
	if resp[0].CertSerial == nil || *resp[0].CertSerial != "123456" {
		t.Errorf("expected cert_serial 123456")
	}

	// Check second node.
	if resp[1].ID != node2ID.String() {
		t.Errorf("expected id %s, got %s", node2ID.String(), resp[1].ID)
	}
	if resp[1].Name != "worker-2" {
		t.Errorf("expected name worker-2, got %s", resp[1].Name)
	}
	if !resp[1].Drained {
		t.Error("expected second node to be drained")
	}
	if resp[1].Version != nil {
		t.Errorf("expected no version for second node")
	}

	if !st.listNodesCalled {
		t.Error("expected ListNodes to be called")
	}
}

// TestListNodesHandlerEmpty verifies empty list when no nodes exist.
func TestListNodesHandlerEmpty(t *testing.T) {
	st := &mockStore{
		listNodesResult: []store.Node{},
	}

	handler := listNodesHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]interface{}](t, rr)

	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp))
	}
}
