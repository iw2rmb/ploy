package lifecycle

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNewCollector_RejectsEmptyNodeID(t *testing.T) {
	_, err := NewCollector(Options{
		Role:   "node",
		NodeID: domaintypes.NodeID(""),
	})
	if err == nil {
		t.Fatal("expected error for empty node id")
	}
}

func TestNewCollector_AcceptsNodeID(t *testing.T) {
	c, err := NewCollector(Options{
		Role:   "node",
		NodeID: domaintypes.NodeID("test-node-123"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.nodeID != domaintypes.NodeID("test-node-123") {
		t.Fatalf("nodeID=%q want %q", c.nodeID, domaintypes.NodeID("test-node-123"))
	}
}
