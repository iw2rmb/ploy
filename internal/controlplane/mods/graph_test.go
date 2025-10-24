package mods

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/mods/plan"
)

func TestValidateStageGraphAcceptsAcyclicGraph(t *testing.T) {
	graph := []StageDefinition{
		{ID: plan.StageNamePlan},
		{ID: plan.StageNameORWApply, Dependencies: []string{plan.StageNamePlan}},
		{ID: plan.StageNameORWGenerate, Dependencies: []string{plan.StageNamePlan}},
		{ID: plan.StageNameLLMExec, Dependencies: []string{plan.StageNameORWApply, plan.StageNameORWGenerate}},
	}
	if err := ValidateStageGraph(graph); err != nil {
		t.Fatalf("expected graph valid, got error: %v", err)
	}
}

func TestValidateStageGraphRejectsDuplicateStageIDs(t *testing.T) {
	graph := []StageDefinition{
		{ID: plan.StageNamePlan},
		{ID: plan.StageNamePlan},
	}
	if err := ValidateStageGraph(graph); err == nil {
		t.Fatalf("expected duplicate stage id to error")
	}
}

func TestValidateStageGraphRejectsUnknownDependency(t *testing.T) {
	graph := []StageDefinition{
		{ID: plan.StageNamePlan},
		{ID: plan.StageNameLLMExec, Dependencies: []string{"unknown-stage"}},
	}
	if err := ValidateStageGraph(graph); err == nil {
		t.Fatalf("expected unknown dependency to error")
	}
}

func TestValidateStageGraphRejectsCycles(t *testing.T) {
	graph := []StageDefinition{
		{ID: "stage-a", Dependencies: []string{"stage-b"}},
		{ID: "stage-b", Dependencies: []string{"stage-a"}},
	}
	if err := ValidateStageGraph(graph); err == nil {
		t.Fatalf("expected cycle detection error")
	}
}
