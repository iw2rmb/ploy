package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListNodeActionsHandler(t *testing.T) {
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	actionID := domaintypes.NewJobID()
	now := time.Now().UTC()
	st := &nodeStore{}
	st.listNodeActions.val = []store.NodeAction{{
		ID:         actionID,
		NodeID:     nodeID,
		ActionType: "node.cleanup_disk",
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
