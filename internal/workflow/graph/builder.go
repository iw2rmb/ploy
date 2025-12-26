package graph

import (
	"errors"
	"fmt"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

var ErrInvalidStepIndex = errors.New("invalid step index")

// BuildFromJobs materializes a WorkflowGraph from a slice of jobs rows.
// This is the primary entry point for constructing the graph view from
// existing persistence. The graph is built in two phases:
//
//  1. Create nodes from jobs (ID, name, type, status, step_index, etc.)
//  2. Compute edges by deriving parent/child relationships from step_index order
//
// The runID parameter identifies the Mods run; it's included in the
// graph for context. All jobs should belong to the same run.
//
// runID is now a domaintypes.RunID (KSUID-backed domain type); no UUID conversion needed.
func BuildFromJobs(runID domaintypes.RunID, jobs []store.Job) (*WorkflowGraph, error) {
	graph := NewWorkflowGraph(runID)

	// Phase 1: Create nodes from jobs.
	for _, job := range jobs {
		node, err := jobToNode(job)
		if err != nil {
			return nil, err
		}
		graph.AddNode(node)
	}

	// Phase 2: Compute edges from step_index ordering.
	graph.ComputeEdges()

	return graph, nil
}

// jobToNode converts a store.Job to a GraphNode.
// Maps database fields to graph node properties.
// job.ID is now a string (KSUID-backed); no UUID conversion needed.
func jobToNode(job store.Job) (*GraphNode, error) {
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

	// Convert raw float64 from store.Job to domaintypes.StepIndex.
	// This centralizes the type boundary between persistence (float64) and
	// domain logic (domaintypes.StepIndex with validation invariants).
	stepIndex := domaintypes.StepIndex(job.StepIndex)
	if !stepIndex.Valid() {
		return nil, fmt.Errorf("job %q has invalid step_index %v: %w", job.ID, job.StepIndex, ErrInvalidStepIndex)
	}

	return &GraphNode{
		ID:         job.ID, // job.ID is now a string (KSUID-backed).
		Name:       job.Name,
		Type:       nodeType,
		StepIndex:  stepIndex,
		Status:     nodeStatus,
		Attempt:    1, // Currently fixed; future retry support may vary.
		Image:      job.ModImage,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMs: job.DurationMs,
		ExitCode:   job.ExitCode,
		ParentIDs:  []string{},
		ChildIDs:   []string{},
	}, nil
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
// strategy. This is provided for future extensibility. Currently delegates to
// the standard linear edge computation. runID is now a domaintypes.RunID.
func BuildFromJobsWithEdgeStrategy(runID domaintypes.RunID, jobs []store.Job, strategy EdgeStrategy) (*WorkflowGraph, error) {
	graph, err := BuildFromJobs(runID, jobs)
	if err != nil {
		return nil, err
	}

	// If a custom strategy is provided, re-compute edges.
	if strategy != nil {
		strategy.ComputeEdges(graph)
	}

	return graph, nil
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
type HealingWindowEdgeStrategy struct{}

// ComputeEdges derives edges considering healing window semantics.
// Currently delegates to linear computation.
func (s *HealingWindowEdgeStrategy) ComputeEdges(g *WorkflowGraph) {
	// For now, use linear edge computation.
	// Future: detect healing windows and create branching edges.
	g.ComputeEdges()
}
