package handlers

import (
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
			wantDrainedFlag: ptr(true),
		},
		{
			name:            "undrain/success",
			handler:         undrainNodeHandler,
			action:          "undrain",
			initialDrained:  true,
			wantStatus:      http.StatusNoContent,
			wantDrainCalled: true,
			wantDrainedFlag: ptr(false),
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

// TestListNodesHandler verifies node listing serialization for populated and empty results.
func TestListNodesHandler(t *testing.T) {
	node1ID := domaintypes.NodeID(domaintypes.NewNodeKey())
	node2ID := domaintypes.NodeID(domaintypes.NewNodeKey())
	now := time.Now()

	cases := []struct {
		name   string
		nodes  []store.Node
		assert func(t *testing.T, resp []map[string]any)
	}{
		{
			name: "two_nodes",
			nodes: []store.Node{
				{
					ID:              node1ID,
					Name:            "worker-1",
					IpAddress:       netip.MustParseAddr("10.0.0.1"),
					Version:         ptr("v1.0.0"),
					Concurrency:     4,
					CpuTotalMillis:  4000,
					CpuFreeMillis:   2000,
					MemTotalBytes:   8589934592,
					MemFreeBytes:    4294967296,
					DiskTotalBytes:  107374182400,
					DiskFreeBytes:   53687091200,
					CertSerial:      ptr("123456"),
					CertFingerprint: ptr("aa:bb:cc"),
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
			assert: func(t *testing.T, resp []map[string]any) {
				t.Helper()
				if len(resp) != 2 {
					t.Fatalf("expected 2 nodes, got %d", len(resp))
				}
				// First node: full fields including optional version and cert.
				if got := resp[0]["id"]; got != node1ID.String() {
					t.Errorf("node[0].id = %v, want %s", got, node1ID)
				}
				if got := resp[0]["name"]; got != "worker-1" {
					t.Errorf("node[0].name = %v, want worker-1", got)
				}
				if got := resp[0]["ip_address"]; got != "10.0.0.1" {
					t.Errorf("node[0].ip_address = %v, want 10.0.0.1", got)
				}
				if got := resp[0]["drained"]; got != false {
					t.Errorf("node[0].drained = %v, want false", got)
				}
				if got := resp[0]["version"]; got != "v1.0.0" {
					t.Errorf("node[0].version = %v, want v1.0.0", got)
				}
				if got := resp[0]["cert_serial"]; got != "123456" {
					t.Errorf("node[0].cert_serial = %v, want 123456", got)
				}
				// Second node: drained, no optional fields.
				if got := resp[1]["id"]; got != node2ID.String() {
					t.Errorf("node[1].id = %v, want %s", got, node2ID)
				}
				if got := resp[1]["name"]; got != "worker-2" {
					t.Errorf("node[1].name = %v, want worker-2", got)
				}
				if got := resp[1]["drained"]; got != true {
					t.Errorf("node[1].drained = %v, want true", got)
				}
				if got, ok := resp[1]["version"]; ok && got != nil {
					t.Errorf("node[1].version = %v, want nil", got)
				}
			},
		},
		{
			name:  "empty",
			nodes: []store.Node{},
			assert: func(t *testing.T, resp []map[string]any) {
				t.Helper()
				if len(resp) != 0 {
					t.Fatalf("expected empty list, got %d items", len(resp))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &mockStore{listNodesResult: tc.nodes}
			rr := doRequest(t, listNodesHandler(st), http.MethodGet, "/v1/nodes", nil)
			assertStatus(t, rr, http.StatusOK)
			resp := decodeBody[[]map[string]any](t, rr)
			tc.assert(t, resp)
		})
	}
}
