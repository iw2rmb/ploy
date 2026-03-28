package handlers

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestDrainUndrain covers success, not-found, and conflict cases for both drain and undrain.
func TestDrainUndrain(t *testing.T) {
	cases := []struct {
		name             string
		handler          func(store.Store) http.HandlerFunc
		action           string
		nodeErr          error
		initialDrained   bool
		wantStatus       int
		wantBodyContains string
		wantDrainCalled  bool
		wantDrainedFlag  *bool
	}{
		{
			name:            "drain/success",
			handler:         drainNodeHandler,
			action:          "drain",
			initialDrained:  false,
			wantStatus:      http.StatusNoContent,
			wantDrainCalled: true,
			wantDrainedFlag: boolPtr(true),
		},
		{
			name:            "undrain/success",
			handler:         undrainNodeHandler,
			action:          "undrain",
			initialDrained:  true,
			wantStatus:      http.StatusNoContent,
			wantDrainCalled: true,
			wantDrainedFlag: boolPtr(false),
		},
		{
			name:             "drain/not_found",
			handler:          drainNodeHandler,
			action:           "drain",
			nodeErr:          pgx.ErrNoRows,
			wantStatus:       http.StatusNotFound,
			wantBodyContains: "node not found",
		},
		{
			name:             "undrain/not_found",
			handler:          undrainNodeHandler,
			action:           "undrain",
			nodeErr:          pgx.ErrNoRows,
			wantStatus:       http.StatusNotFound,
			wantBodyContains: "node not found",
		},
		{
			name:             "drain/already_drained",
			handler:          drainNodeHandler,
			action:           "drain",
			initialDrained:   true,
			wantStatus:       http.StatusConflict,
			wantBodyContains: "already drained",
		},
		{
			name:             "undrain/not_drained",
			handler:          undrainNodeHandler,
			action:           "undrain",
			initialDrained:   false,
			wantStatus:       http.StatusConflict,
			wantBodyContains: "not drained",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodeIDStr, node := newNodeFixture(tc.initialDrained)
			st := &mockStore{
				getNodeResult: node,
				getNodeErr:    tc.nodeErr,
			}

			h := tc.handler(st)
			rr := doRequest(t, h, http.MethodPost, "/v1/nodes/"+nodeIDStr+"/"+tc.action, nil, "id", nodeIDStr)

			assertStatus(t, rr, tc.wantStatus)

			if tc.wantBodyContains != "" && !strings.Contains(rr.Body.String(), tc.wantBodyContains) {
				t.Errorf("expected body to contain %q, got: %s", tc.wantBodyContains, rr.Body.String())
			}
			if tc.wantDrainCalled {
				assertCalled(t, "UpdateNodeDrained", st.updateNodeDrainedCalled)
			} else {
				assertNotCalled(t, "UpdateNodeDrained", st.updateNodeDrainedCalled)
			}
			if tc.wantDrainedFlag != nil && st.updateNodeDrainedParams.Drained != *tc.wantDrainedFlag {
				t.Errorf("expected drained flag %v, got %v", *tc.wantDrainedFlag, st.updateNodeDrainedParams.Drained)
			}
		})
	}
}

// TestDrainUndrain_InvalidID verifies rejection of invalid node IDs for both drain and undrain.
func TestDrainUndrain_InvalidID(t *testing.T) {
	handlers := []struct {
		name    string
		handler func(store.Store) http.HandlerFunc
		action  string
	}{
		{"drain", drainNodeHandler, "drain"},
		{"undrain", undrainNodeHandler, "undrain"},
	}
	ids := []struct {
		name  string
		id    string
		urlID string
	}{
		{"empty id", "", "x"},
		{"whitespace", "   ", "x"},
		{"invalid nanoid", "not-a-nanoid", "x"},
	}

	for _, h := range handlers {
		for _, tc := range ids {
			t.Run(h.name+"/"+tc.name, func(t *testing.T) {
				st := &mockStore{}
				rr := doRequest(t, h.handler(st), http.MethodPost, "/v1/nodes/"+tc.urlID+"/"+h.action, nil, "id", tc.id)
				assertStatus(t, rr, http.StatusBadRequest)
			})
		}
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
	rr := doRequest(t, handler, http.MethodGet, "/v1/nodes", nil)

	assertStatus(t, rr, http.StatusOK)
	assertCalled(t, "ListNodes", st.listNodesCalled)

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
}

// TestListNodesHandlerEmpty verifies empty list when no nodes exist.
func TestListNodesHandlerEmpty(t *testing.T) {
	st := &mockStore{
		listNodesResult: []store.Node{},
	}

	handler := listNodesHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/nodes", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]interface{}](t, rr)
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp))
	}
}
