package mods

import (
	"fmt"
)

// stageGraph models directed dependencies between stages.
type stageGraph struct {
	stages     map[string]StageDefinition
	forward    map[string][]string
	reverse    map[string][]string
	rootStages []string
}

// ValidateStageGraph ensures the provided stage definitions form a valid DAG.
func ValidateStageGraph(stages []StageDefinition) error {
	_, err := buildStageGraph(stages)
	return err
}

// buildStageGraph constructs the stage dependency graph and validates correctness.
func buildStageGraph(stages []StageDefinition) (*stageGraph, error) {
	graph := &stageGraph{
		stages:     make(map[string]StageDefinition),
		forward:    make(map[string][]string),
		reverse:    make(map[string][]string),
		rootStages: make([]string, 0),
	}
	for _, stage := range stages {
		if stage.ID == "" {
			return nil, fmt.Errorf("stage id must be set")
		}
		if _, exists := graph.stages[stage.ID]; exists {
			return nil, fmt.Errorf("duplicate stage id %q", stage.ID)
		}
		graph.stages[stage.ID] = stage
		graph.forward[stage.ID] = append([]string{}, stage.Dependencies...)
		for _, dep := range stage.Dependencies {
			graph.reverse[dep] = append(graph.reverse[dep], stage.ID)
		}
	}
	for id := range graph.stages {
		if _, ok := graph.reverse[id]; !ok {
			graph.reverse[id] = nil
		}
	}
	for id := range graph.forward {
		if len(graph.forward[id]) == 0 {
			graph.rootStages = append(graph.rootStages, id)
		}
	}
	if len(graph.stages) == 0 {
		return nil, fmt.Errorf("stage graph must contain at least one stage")
	}
	for id, deps := range graph.forward {
		for _, dep := range deps {
			if _, ok := graph.stages[dep]; !ok {
				return nil, fmt.Errorf("stage %q depends on unknown stage %q", id, dep)
			}
		}
	}
	if hasCycle(graph.forward) {
		return nil, fmt.Errorf("stage graph contains a cycle")
	}
	return graph, nil
}

// roots returns stage identifiers without dependencies.
// roots returns identifiers for stages without inbound dependencies.
func (g *stageGraph) roots() []string {
	return append([]string{}, g.rootStages...)
}

// dependents returns the downstream stages for the provided stage identifier.
func (g *stageGraph) dependents(stageID string) []string {
	return append([]string{}, g.reverse[stageID]...)
}

// dependencies returns the upstream dependencies for the provided stage identifier.
func (g *stageGraph) dependencies(stageID string) []string {
	return append([]string{}, g.forward[stageID]...)
}

// hasCycle detects cycles via depth-first search.
func hasCycle(edges map[string][]string) bool {
	visited := make(map[string]bool)
	onStack := make(map[string]bool)

	var visit func(node string) bool
	visit = func(node string) bool {
		if onStack[node] {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		onStack[node] = true
		for _, dep := range edges[node] {
			if visit(dep) {
				return true
			}
		}
		onStack[node] = false
		return false
	}

	for node := range edges {
		if !visited[node] {
			if visit(node) {
				return true
			}
		}
	}
	return false
}
