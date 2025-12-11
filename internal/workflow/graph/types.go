// Package graph provides a workflow graph view for Mods runs. It treats jobs
// as explicit nodes and exposes dependencies derived from step_index ordering.
// The graph is materialized from existing jobs rows without requiring additional
// persistence; any serialized view is optional and debug-focused.
//
// Background: Mods runs execute as an ordered sequence of jobs (pre_gate, mod,
// heal, re_gate, post_gate) where step_index determines execution order. This
// package formalizes the implicit parent/child relationships into an explicit
// graph structure for visualization and debugging.
package graph

import (
	"time"
)

// NodeType identifies the job phase within a Mods workflow.
// These values match the mod_type field in the jobs table.
type NodeType string

const (
	// NodeTypePreGate represents pre-mod validation/gate jobs.
	NodeTypePreGate NodeType = "pre_gate"
	// NodeTypeMod represents main modification execution jobs.
	NodeTypeMod NodeType = "mod"
	// NodeTypeHeal represents healing jobs inserted after gate failures.
	NodeTypeHeal NodeType = "heal"
	// NodeTypeReGate represents re-validation jobs after healing.
	NodeTypeReGate NodeType = "re_gate"
	// NodeTypePostGate represents post-mod validation jobs.
	NodeTypePostGate NodeType = "post_gate"
)

// NodeStatus represents the execution state of a graph node.
// These values align with the job_status enum in the database.
type NodeStatus string

const (
	// NodeStatusCreated indicates the job is created but not yet scheduled.
	NodeStatusCreated NodeStatus = "created"
	// NodeStatusPending indicates the job is ready to be claimed.
	NodeStatusPending NodeStatus = "pending"
	// NodeStatusRunning indicates the job is currently executing.
	NodeStatusRunning NodeStatus = "running"
	// NodeStatusSucceeded indicates the job completed successfully.
	NodeStatusSucceeded NodeStatus = "succeeded"
	// NodeStatusFailed indicates the job execution failed.
	NodeStatusFailed NodeStatus = "failed"
	// NodeStatusSkipped indicates the job was skipped (not executed).
	NodeStatusSkipped NodeStatus = "skipped"
	// NodeStatusCanceled indicates the job was canceled.
	NodeStatusCanceled NodeStatus = "canceled"
)

// GraphNode represents a single job as a node in the workflow graph.
// Nodes are connected by edges derived from step_index ordering and
// gate/healing window relationships.
type GraphNode struct {
	// ID is the unique job identifier (KSUID string).
	ID string `json:"id"`

	// Name is the job name (e.g., "pre-gate", "mod-0", "heal-1", "re-gate").
	Name string `json:"name"`

	// Type identifies the job phase (pre_gate, mod, heal, re_gate, post_gate).
	Type NodeType `json:"type"`

	// StepIndex is the float ordering value from jobs.step_index.
	// Values like 1000, 2000, 3000 define main job sequence; midpoints
	// (e.g., 1500, 1750) are used for dynamically inserted healing jobs.
	StepIndex float64 `json:"step_index"`

	// Status is the current execution state of the job.
	Status NodeStatus `json:"status"`

	// Attempt indicates the retry attempt number (currently always 1).
	// Future retry support may increment this value.
	Attempt int `json:"attempt"`

	// ParentIDs lists IDs of predecessor nodes (jobs that must complete first).
	// Derived from step_index ordering: nodes with lower step_index are parents.
	ParentIDs []string `json:"parent_ids,omitempty"`

	// ChildIDs lists IDs of successor nodes (jobs that depend on this one).
	// Derived from step_index ordering: nodes with higher step_index are children.
	ChildIDs []string `json:"child_ids,omitempty"`

	// Image is the container image for this job (optional, for diagnostics).
	Image string `json:"image,omitempty"`

	// StartedAt is when the job started execution.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// FinishedAt is when the job finished execution.
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// DurationMs is the execution duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`

	// ExitCode is the job's exit code (if available).
	ExitCode *int32 `json:"exit_code,omitempty"`
}

// WorkflowGraph represents the complete job graph for a Mods run.
// It provides an explicit view of job dependencies materialized from
// jobs rows using step_index ordering.
type WorkflowGraph struct {
	// RunID is the run identifier (KSUID string).
	RunID string `json:"run_id"`

	// Nodes contains all jobs as graph nodes, keyed by job ID.
	Nodes map[string]*GraphNode `json:"nodes"`

	// RootIDs lists IDs of entry-point nodes (jobs with no parents).
	// Typically contains only the pre-gate job.
	RootIDs []string `json:"root_ids"`

	// LeafIDs lists IDs of terminal nodes (jobs with no children).
	// Typically contains only the post-gate job (or last job in sequence).
	LeafIDs []string `json:"leaf_ids"`

	// Linear indicates whether the graph is a simple linear chain.
	// True for standard runs (pre-gate → mod → post-gate). Reserved for
	// future non-linear graph support.
	Linear bool `json:"linear"`
}

// NewWorkflowGraph creates an empty WorkflowGraph for the given run ID.
func NewWorkflowGraph(runID string) *WorkflowGraph {
	return &WorkflowGraph{
		RunID:   runID,
		Nodes:   make(map[string]*GraphNode),
		RootIDs: []string{},
		LeafIDs: []string{},
		Linear:  true,
	}
}

// AddNode adds a node to the graph. Does not compute edges; call
// ComputeEdges after all nodes are added.
func (g *WorkflowGraph) AddNode(node *GraphNode) {
	if node == nil {
		return
	}
	// Initialize slice fields to avoid nil in JSON output.
	if node.ParentIDs == nil {
		node.ParentIDs = []string{}
	}
	if node.ChildIDs == nil {
		node.ChildIDs = []string{}
	}
	g.Nodes[node.ID] = node
}

// ComputeEdges derives parent/child relationships from step_index ordering.
// Current implementation: linear chain derived from step_index order. Each
// node's parent is the node with the next-lowest step_index.
func (g *WorkflowGraph) ComputeEdges() {
	if len(g.Nodes) == 0 {
		return
	}

	// Build sorted list of nodes by step_index.
	sorted := g.sortedNodes()

	// Clear existing edges and set up parent/child relationships.
	for _, node := range g.Nodes {
		node.ParentIDs = []string{}
		node.ChildIDs = []string{}
	}

	// Linear chain: each node's parent is the previous node in sorted order.
	for i := 1; i < len(sorted); i++ {
		parent := sorted[i-1]
		child := sorted[i]
		parent.ChildIDs = append(parent.ChildIDs, child.ID)
		child.ParentIDs = append(child.ParentIDs, parent.ID)
	}

	// Identify roots (no parents) and leaves (no children).
	g.RootIDs = []string{}
	g.LeafIDs = []string{}
	for _, node := range sorted {
		if len(node.ParentIDs) == 0 {
			g.RootIDs = append(g.RootIDs, node.ID)
		}
		if len(node.ChildIDs) == 0 {
			g.LeafIDs = append(g.LeafIDs, node.ID)
		}
	}

	// Determine linearity: graph is linear if every node has at most
	// one parent and one child.
	g.Linear = true
	for _, node := range g.Nodes {
		if len(node.ParentIDs) > 1 || len(node.ChildIDs) > 1 {
			g.Linear = false
			break
		}
	}
}

// sortedNodes returns nodes sorted by step_index ascending.
func (g *WorkflowGraph) sortedNodes() []*GraphNode {
	// Collect all nodes.
	nodes := make([]*GraphNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}

	// Sort by step_index using insertion sort (small N, stable).
	for i := 1; i < len(nodes); i++ {
		key := nodes[i]
		j := i - 1
		for j >= 0 && nodes[j].StepIndex > key.StepIndex {
			nodes[j+1] = nodes[j]
			j--
		}
		nodes[j+1] = key
	}
	return nodes
}

// GetNode returns the node with the given ID, or nil if not found.
func (g *WorkflowGraph) GetNode(id string) *GraphNode {
	return g.Nodes[id]
}

// NodeCount returns the total number of nodes in the graph.
func (g *WorkflowGraph) NodeCount() int {
	return len(g.Nodes)
}

// OrderedNodes returns nodes in execution order (sorted by step_index).
func (g *WorkflowGraph) OrderedNodes() []*GraphNode {
	return g.sortedNodes()
}

// IsTerminal returns true if the given node ID is a leaf (no children).
func (g *WorkflowGraph) IsTerminal(id string) bool {
	node := g.Nodes[id]
	if node == nil {
		return false
	}
	return len(node.ChildIDs) == 0
}

// IsGateNode returns true if the node represents a gate job
// (pre_gate, post_gate, or re_gate).
func (n *GraphNode) IsGateNode() bool {
	switch n.Type {
	case NodeTypePreGate, NodeTypePostGate, NodeTypeReGate:
		return true
	default:
		return false
	}
}

// IsHealingNode returns true if the node represents a healing job.
func (n *GraphNode) IsHealingNode() bool {
	return n.Type == NodeTypeHeal
}
