package graph

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestNewWorkflowGraph verifies that NewWorkflowGraph initializes all fields.
func TestNewWorkflowGraph(t *testing.T) {
	t.Parallel()

	runID := "test-run-123"
	g := NewWorkflowGraph(domaintypes.RunID(runID))

	if g.RunID.String() != runID {
		t.Errorf("RunID = %q, want %q", g.RunID.String(), runID)
	}
	if g.Nodes == nil {
		t.Error("Nodes should be initialized, got nil")
	}
	if g.RootIDs == nil {
		t.Error("RootIDs should be initialized, got nil")
	}
	if g.LeafIDs == nil {
		t.Error("LeafIDs should be initialized, got nil")
	}
	if !g.Linear {
		t.Error("Linear should default to true")
	}
}

// TestAddNode verifies that AddNode adds nodes and initializes slice fields.
func TestAddNode(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))

	// Add node with nil slices (should be initialized).
	node := &GraphNode{
		ID:        "job-1",
		Name:      "pre-gate",
		Type:      NodeTypePreGate,
		StepIndex: 1000,
		Status:    NodeStatusPending,
	}
	g.AddNode(node)

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
	if g.Nodes["job-1"] == nil {
		t.Error("node 'job-1' should be present")
	}
	if node.ParentIDs == nil {
		t.Error("ParentIDs should be initialized to empty slice")
	}
	if node.ChildIDs == nil {
		t.Error("ChildIDs should be initialized to empty slice")
	}

	// Adding nil node should be a no-op.
	g.AddNode(nil)
	if len(g.Nodes) != 1 {
		t.Errorf("adding nil should not change node count, got %d", len(g.Nodes))
	}
}

// TestComputeEdges_LinearChain verifies edge computation for a simple linear chain.
func TestComputeEdges_LinearChain(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))

	// Create a standard 3-job pipeline: pre-gate → mod-0 → post-gate.
	g.AddNode(&GraphNode{ID: "pre-gate", Name: "pre-gate", Type: NodeTypePreGate, StepIndex: 1000, Status: NodeStatusSucceeded})
	g.AddNode(&GraphNode{ID: "mod-0", Name: "mod-0", Type: NodeTypeMod, StepIndex: 2000, Status: NodeStatusRunning})
	g.AddNode(&GraphNode{ID: "post-gate", Name: "post-gate", Type: NodeTypePostGate, StepIndex: 3000, Status: NodeStatusCreated})

	g.ComputeEdges()

	// Verify edges.
	preGate := g.Nodes["pre-gate"]
	mod0 := g.Nodes["mod-0"]
	postGate := g.Nodes["post-gate"]

	// pre-gate should have no parents and one child (mod-0).
	if len(preGate.ParentIDs) != 0 {
		t.Errorf("pre-gate should have 0 parents, got %d", len(preGate.ParentIDs))
	}
	if len(preGate.ChildIDs) != 1 || preGate.ChildIDs[0] != "mod-0" {
		t.Errorf("pre-gate should have child 'mod-0', got %v", preGate.ChildIDs)
	}

	// mod-0 should have one parent (pre-gate) and one child (post-gate).
	if len(mod0.ParentIDs) != 1 || mod0.ParentIDs[0] != "pre-gate" {
		t.Errorf("mod-0 should have parent 'pre-gate', got %v", mod0.ParentIDs)
	}
	if len(mod0.ChildIDs) != 1 || mod0.ChildIDs[0] != "post-gate" {
		t.Errorf("mod-0 should have child 'post-gate', got %v", mod0.ChildIDs)
	}

	// post-gate should have one parent (mod-0) and no children.
	if len(postGate.ParentIDs) != 1 || postGate.ParentIDs[0] != "mod-0" {
		t.Errorf("post-gate should have parent 'mod-0', got %v", postGate.ParentIDs)
	}
	if len(postGate.ChildIDs) != 0 {
		t.Errorf("post-gate should have 0 children, got %d", len(postGate.ChildIDs))
	}

	// Verify roots and leaves.
	if len(g.RootIDs) != 1 || g.RootIDs[0] != "pre-gate" {
		t.Errorf("RootIDs should be ['pre-gate'], got %v", g.RootIDs)
	}
	if len(g.LeafIDs) != 1 || g.LeafIDs[0] != "post-gate" {
		t.Errorf("LeafIDs should be ['post-gate'], got %v", g.LeafIDs)
	}

	// Should be linear.
	if !g.Linear {
		t.Error("graph should be linear")
	}
}

// TestComputeEdges_WithHealing verifies edge computation with healing jobs.
func TestComputeEdges_WithHealing(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))

	// Simulate a healing scenario:
	// pre-gate (1000) → heal-1 (1500) → re-gate (1750) → mod-0 (2000) → post-gate (3000)
	g.AddNode(&GraphNode{ID: "pre-gate", Name: "pre-gate", Type: NodeTypePreGate, StepIndex: 1000, Status: NodeStatusFailed})
	g.AddNode(&GraphNode{ID: "heal-1", Name: "heal-1", Type: NodeTypeHeal, StepIndex: 1500, Status: NodeStatusSucceeded})
	g.AddNode(&GraphNode{ID: "re-gate", Name: "re-gate", Type: NodeTypeReGate, StepIndex: 1750, Status: NodeStatusSucceeded})
	g.AddNode(&GraphNode{ID: "mod-0", Name: "mod-0", Type: NodeTypeMod, StepIndex: 2000, Status: NodeStatusRunning})
	g.AddNode(&GraphNode{ID: "post-gate", Name: "post-gate", Type: NodeTypePostGate, StepIndex: 3000, Status: NodeStatusCreated})

	g.ComputeEdges()

	// Verify linear chain structure.
	heal1 := g.Nodes["heal-1"]
	reGate := g.Nodes["re-gate"]

	if len(heal1.ParentIDs) != 1 || heal1.ParentIDs[0] != "pre-gate" {
		t.Errorf("heal-1 should have parent 'pre-gate', got %v", heal1.ParentIDs)
	}
	if len(heal1.ChildIDs) != 1 || heal1.ChildIDs[0] != "re-gate" {
		t.Errorf("heal-1 should have child 're-gate', got %v", heal1.ChildIDs)
	}

	if len(reGate.ParentIDs) != 1 || reGate.ParentIDs[0] != "heal-1" {
		t.Errorf("re-gate should have parent 'heal-1', got %v", reGate.ParentIDs)
	}

	// Verify total node count.
	if g.NodeCount() != 5 {
		t.Errorf("expected 5 nodes, got %d", g.NodeCount())
	}
}

// TestComputeEdges_EmptyGraph verifies edge computation on empty graph.
func TestComputeEdges_EmptyGraph(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))
	g.ComputeEdges()

	if len(g.RootIDs) != 0 {
		t.Errorf("empty graph should have no roots, got %v", g.RootIDs)
	}
	if len(g.LeafIDs) != 0 {
		t.Errorf("empty graph should have no leaves, got %v", g.LeafIDs)
	}
}

// TestComputeEdges_SingleNode verifies edge computation with a single node.
func TestComputeEdges_SingleNode(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))
	g.AddNode(&GraphNode{ID: "only-job", Name: "only-job", Type: NodeTypeMod, StepIndex: 1000, Status: NodeStatusPending})
	g.ComputeEdges()

	node := g.Nodes["only-job"]
	if len(node.ParentIDs) != 0 {
		t.Errorf("single node should have no parents, got %v", node.ParentIDs)
	}
	if len(node.ChildIDs) != 0 {
		t.Errorf("single node should have no children, got %v", node.ChildIDs)
	}

	// Single node is both root and leaf.
	if len(g.RootIDs) != 1 || g.RootIDs[0] != "only-job" {
		t.Errorf("single node should be root, got %v", g.RootIDs)
	}
	if len(g.LeafIDs) != 1 || g.LeafIDs[0] != "only-job" {
		t.Errorf("single node should be leaf, got %v", g.LeafIDs)
	}
}

// TestOrderedNodes verifies that OrderedNodes returns nodes sorted by step_index.
func TestOrderedNodes(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))

	// Add nodes out of order.
	g.AddNode(&GraphNode{ID: "c", StepIndex: 3000})
	g.AddNode(&GraphNode{ID: "a", StepIndex: 1000})
	g.AddNode(&GraphNode{ID: "b", StepIndex: 2000})

	ordered := g.OrderedNodes()

	if len(ordered) != 3 {
		t.Fatalf("expected 3 ordered nodes, got %d", len(ordered))
	}
	if ordered[0].ID != "a" || ordered[1].ID != "b" || ordered[2].ID != "c" {
		t.Errorf("nodes should be ordered a,b,c, got %s,%s,%s", ordered[0].ID, ordered[1].ID, ordered[2].ID)
	}
}

// TestGetNode verifies node lookup by ID.
func TestGetNode(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))
	g.AddNode(&GraphNode{ID: "job-1", Name: "test-job"})

	if n := g.GetNode("job-1"); n == nil || n.Name != "test-job" {
		t.Error("GetNode should return the node by ID")
	}
	if n := g.GetNode("nonexistent"); n != nil {
		t.Error("GetNode should return nil for nonexistent ID")
	}
}

// TestIsTerminal verifies terminal node detection.
func TestIsTerminal(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))
	g.AddNode(&GraphNode{ID: "a", StepIndex: 1000})
	g.AddNode(&GraphNode{ID: "b", StepIndex: 2000})
	g.ComputeEdges()

	if g.IsTerminal("a") {
		t.Error("'a' should not be terminal (has child)")
	}
	if !g.IsTerminal("b") {
		t.Error("'b' should be terminal (no children)")
	}
	if g.IsTerminal("nonexistent") {
		t.Error("nonexistent node should not be terminal")
	}
}

// TestGraphNode_IsGateNode verifies gate node type detection.
func TestGraphNode_IsGateNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nodeType NodeType
		isGate   bool
	}{
		{NodeTypePreGate, true},
		{NodeTypePostGate, true},
		{NodeTypeReGate, true},
		{NodeTypeMod, false},
		{NodeTypeHeal, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.nodeType), func(t *testing.T) {
			t.Parallel()
			node := &GraphNode{Type: tt.nodeType}
			if node.IsGateNode() != tt.isGate {
				t.Errorf("IsGateNode() = %v, want %v", node.IsGateNode(), tt.isGate)
			}
		})
	}
}

// TestGraphNode_IsHealingNode verifies healing node type detection.
func TestGraphNode_IsHealingNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nodeType  NodeType
		isHealing bool
	}{
		{NodeTypeHeal, true},
		{NodeTypePreGate, false},
		{NodeTypeMod, false},
		{NodeTypeReGate, false},
		{NodeTypePostGate, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.nodeType), func(t *testing.T) {
			t.Parallel()
			node := &GraphNode{Type: tt.nodeType}
			if node.IsHealingNode() != tt.isHealing {
				t.Errorf("IsHealingNode() = %v, want %v", node.IsHealingNode(), tt.isHealing)
			}
		})
	}
}

// TestWorkflowGraph_JSONSerialization verifies that the graph serializes to JSON correctly.
func TestWorkflowGraph_JSONSerialization(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-123"))
	g.AddNode(&GraphNode{
		ID:        "job-1",
		Name:      "pre-gate",
		Type:      NodeTypePreGate,
		StepIndex: 1000,
		Status:    NodeStatusSucceeded,
		Attempt:   1,
	})
	g.AddNode(&GraphNode{
		ID:        "job-2",
		Name:      "mod-0",
		Type:      NodeTypeMod,
		StepIndex: 2000,
		Status:    NodeStatusRunning,
		Attempt:   1,
	})
	g.ComputeEdges()

	// Serialize to JSON.
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("failed to marshal graph: %v", err)
	}

	// Verify key fields are present.
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal graph: %v", err)
	}

	if result["run_id"] != "run-123" {
		t.Errorf("run_id should be 'run-123', got %v", result["run_id"])
	}
	if result["linear"] != true {
		t.Errorf("linear should be true, got %v", result["linear"])
	}

	nodes, ok := result["nodes"].(map[string]interface{})
	if !ok {
		t.Fatal("nodes should be a map")
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

// TestComputeEdges_RecalculatesEdges verifies that calling ComputeEdges
// clears and recalculates edges.
func TestComputeEdges_RecalculatesEdges(t *testing.T) {
	t.Parallel()

	g := NewWorkflowGraph(domaintypes.RunID("run-1"))
	g.AddNode(&GraphNode{ID: "a", StepIndex: 1000, ChildIDs: []string{"stale"}})
	g.AddNode(&GraphNode{ID: "b", StepIndex: 2000, ParentIDs: []string{"stale"}})
	g.ComputeEdges()

	// Verify stale edges are cleared.
	a := g.Nodes["a"]
	b := g.Nodes["b"]

	if len(a.ChildIDs) != 1 || a.ChildIDs[0] != "b" {
		t.Errorf("'a' should have child 'b', got %v", a.ChildIDs)
	}
	if len(b.ParentIDs) != 1 || b.ParentIDs[0] != "a" {
		t.Errorf("'b' should have parent 'a', got %v", b.ParentIDs)
	}
}
