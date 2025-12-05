package graph

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// BuildFromJobs materializes a WorkflowGraph from a slice of jobs rows.
// This is the primary entry point for constructing the graph view from
// existing persistence. The graph is built in two phases:
//
//  1. Create nodes from jobs (ID, name, type, status, step_index, etc.)
//  2. Compute edges by deriving parent/child relationships from step_index order
//
// The runID parameter identifies the ticket/run; it's included in the
// graph for context. All jobs should belong to the same run.
func BuildFromJobs(runID pgtype.UUID, jobs []store.Job) *WorkflowGraph {
	// Convert runID to string for JSON-friendly output.
	runIDStr := ""
	if runID.Valid {
		runIDStr = uuid.UUID(runID.Bytes).String()
	}

	graph := NewWorkflowGraph(runIDStr)

	// Phase 1: Create nodes from jobs.
	for _, job := range jobs {
		node := jobToNode(job)
		graph.AddNode(node)
	}

	// Phase 2: Compute edges from step_index ordering.
	graph.ComputeEdges()

	return graph
}

// jobToNode converts a store.Job to a GraphNode.
// Maps database fields to graph node properties.
func jobToNode(job store.Job) *GraphNode {
	// Convert job ID to string.
	jobID := ""
	if job.ID.Valid {
		jobID = uuid.UUID(job.ID.Bytes).String()
	}

	// Map job status to node status.
	nodeStatus := mapJobStatus(job.Status)

	// Map mod_type to node type.
	nodeType := mapModType(job.ModType)

	// Convert timestamps.
	var startedAt, finishedAt *time.Time
	if job.StartedAt.Valid {
		t := job.StartedAt.Time
		startedAt = &t
	}
	if job.FinishedAt.Valid {
		t := job.FinishedAt.Time
		finishedAt = &t
	}

	return &GraphNode{
		ID:         jobID,
		Name:       job.Name,
		Type:       nodeType,
		StepIndex:  job.StepIndex,
		Status:     nodeStatus,
		Attempt:    1, // Currently fixed; future retry support may vary.
		Image:      job.ModImage,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: job.DurationMs,
		ExitCode:   job.ExitCode,
		ParentIDs:  []string{},
		ChildIDs:   []string{},
	}
}

// mapJobStatus converts store.JobStatus to NodeStatus.
func mapJobStatus(status store.JobStatus) NodeStatus {
	switch status {
	case store.JobStatusCreated:
		return NodeStatusCreated
	case store.JobStatusPending:
		return NodeStatusPending
	case store.JobStatusRunning:
		return NodeStatusRunning
	case store.JobStatusSucceeded:
		return NodeStatusSucceeded
	case store.JobStatusFailed:
		return NodeStatusFailed
	case store.JobStatusSkipped:
		return NodeStatusSkipped
	case store.JobStatusCanceled:
		return NodeStatusCanceled
	default:
		// Fallback for unknown status; treat as created.
		return NodeStatusCreated
	}
}

// mapModType converts the mod_type string to NodeType.
func mapModType(modType string) NodeType {
	switch modType {
	case "pre_gate":
		return NodeTypePreGate
	case "mod":
		return NodeTypeMod
	case "heal":
		return NodeTypeHeal
	case "re_gate":
		return NodeTypeReGate
	case "post_gate":
		return NodeTypePostGate
	default:
		// Fallback for unknown type; treat as mod.
		return NodeTypeMod
	}
}

// BuildFromJobsWithEdgeStrategy allows specifying a custom edge computation
// strategy. This is provided for future extensibility (e.g., parallel healing
// branches). Currently delegates to the standard linear edge computation.
func BuildFromJobsWithEdgeStrategy(runID pgtype.UUID, jobs []store.Job, strategy EdgeStrategy) *WorkflowGraph {
	graph := BuildFromJobs(runID, jobs)

	// If a custom strategy is provided, re-compute edges.
	if strategy != nil {
		strategy.ComputeEdges(graph)
	}

	return graph
}

// EdgeStrategy defines an interface for custom edge computation algorithms.
// The default implementation uses linear step_index ordering.
type EdgeStrategy interface {
	// ComputeEdges populates parent/child relationships in the graph.
	ComputeEdges(g *WorkflowGraph)
}

// LinearEdgeStrategy implements EdgeStrategy for linear job chains.
// This is the default strategy used by ComputeEdges.
type LinearEdgeStrategy struct{}

// ComputeEdges derives edges from step_index ordering (linear chain).
func (s *LinearEdgeStrategy) ComputeEdges(g *WorkflowGraph) {
	g.ComputeEdges()
}

// HealingWindowEdgeStrategy implements EdgeStrategy for graphs with
// healing windows. It groups healing jobs with their associated gate
// and creates appropriate edge relationships.
//
// Healing window pattern:
//
//	gate (failed) → heal-1 → re-gate → heal-2 → re-gate → ... → next-job
//
// This strategy is provided for future parallel healing support.
type HealingWindowEdgeStrategy struct{}

// ComputeEdges derives edges considering healing window semantics.
// Currently delegates to linear computation; future implementation
// will handle parallel healing branches.
func (s *HealingWindowEdgeStrategy) ComputeEdges(g *WorkflowGraph) {
	// For now, use linear edge computation.
	// Future: detect healing windows and create branching edges.
	g.ComputeEdges()
}
