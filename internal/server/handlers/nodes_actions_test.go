package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateNodeActionHandler(t *testing.T) {
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantCalled bool
	}{
		{name: "cleanup", body: `{"action_type":"node.cleanup_disk"}`, wantStatus: http.StatusCreated, wantCalled: true},
		{name: "update updater", body: `{"action_type":"node.update_updater"}`, wantStatus: http.StatusCreated, wantCalled: true},
		{name: "unsupported", body: `{"action_type":"node.shell"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &nodeStore{}
			st.getNode.val = store.Node{ID: nodeID, Name: "node"}
			rr := doRequest(t, createNodeActionHandler(st), http.MethodPost, "/v1/nodes/"+nodeID.String()+"/actions", tc.body, "id", nodeID.String())
			assertStatus(t, rr, tc.wantStatus)
			if st.createNodeAction.called != tc.wantCalled {
				t.Fatalf("CreateNodeAction called = %v, want %v", st.createNodeAction.called, tc.wantCalled)
			}
			if !tc.wantCalled {
				return
			}
			if st.createNodeAction.params.NodeID != nodeID {
				t.Fatalf("CreateNodeAction node_id = %s, want %s", st.createNodeAction.params.NodeID, nodeID)
			}
			if strings.TrimSpace(st.createNodeAction.params.ActionType) == "" {
				t.Fatal("CreateNodeAction action_type is empty")
			}
		})
	}
}

func TestListNodeActionsHandler(t *testing.T) {
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	actionID := domaintypes.NewJobID()
	now := time.Now().UTC()
	st := &nodeStore{}
	st.listNodeActions.val = []store.NodeAction{{
		ID:         actionID,
		NodeID:     nodeID,
		ActionType: domaintypes.NodeActionCleanupDisk,
		Status:     domaintypes.JobStatusSuccess,
		DurationMs: 12,
		CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	}}

	rr := doRequest(t, listNodeActionsHandler(st), http.MethodGet, "/v1/nodes/"+nodeID.String()+"/actions?limit=5", nil, "id", nodeID.String())
	assertStatus(t, rr, http.StatusOK)
	if st.listNodeActions.params.NodeID != nodeID || st.listNodeActions.params.Limit != 5 {
		t.Fatalf("ListNodeActions params = (%s,%d), want (%s,5)", st.listNodeActions.params.NodeID, st.listNodeActions.params.Limit, nodeID)
	}
	resp := decodeBody[[]map[string]any](t, rr)
	if len(resp) != 1 || resp[0]["id"] != actionID.String() {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
