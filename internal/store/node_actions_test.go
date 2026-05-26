package store

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNodeActions_CreateClaimCompleteList(t *testing.T) {
	ctx, db := newTestStore(t)
	node := createTestNode(t, ctx, db)

	action, err := db.CreateNodeAction(ctx, CreateNodeActionParams{
		ID:         types.NewJobID(),
		NodeID:     node.ID,
		ActionType: "node.cleanup_disk",
		Status:     types.JobStatusQueued,
		Meta:       []byte(`{"reason":"test"}`),
	})
	if err != nil {
		t.Fatalf("CreateNodeAction() error = %v", err)
	}

	claimed, err := db.ClaimNodeAction(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimNodeAction() error = %v", err)
	}
	if claimed.ID != action.ID || claimed.Status != types.JobStatusRunning {
		t.Fatalf("claimed action = (%s,%s), want (%s,%s)", claimed.ID, claimed.Status, action.ID, types.JobStatusRunning)
	}

	if err := db.UpdateNodeActionCompletion(ctx, UpdateNodeActionCompletionParams{
		ID:     action.ID,
		Status: types.JobStatusSuccess,
		Result: []byte(`{"output":"ok"}`),
	}); err != nil {
		t.Fatalf("UpdateNodeActionCompletion() error = %v", err)
	}

	actions, err := db.ListNodeActions(ctx, ListNodeActionsParams{NodeID: node.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListNodeActions() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("ListNodeActions() len = %d, want 1", len(actions))
	}
	if actions[0].Status != types.JobStatusSuccess {
		t.Fatalf("action status = %s, want %s", actions[0].Status, types.JobStatusSuccess)
	}
}
