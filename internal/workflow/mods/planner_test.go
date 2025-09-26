package mods_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

func TestPlannerBuildsModsStageGraph(t *testing.T) {
	planner := mods.NewPlanner(mods.Options{
		PlanLane:        "node-wasm",
		OpenRewriteLane: "node-wasm",
		LLMPlanLane:     "gpu-ml",
		LLMExecLane:     "gpu-ml",
		HumanLane:       "go-native",
	})
	stages, err := planner.Plan(context.Background(), mods.PlanInput{
		Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if len(stages) != 6 {
		t.Fatalf("expected 6 stages, got %d", len(stages))
	}
	expect := []struct {
		name string
		kind string
		lane string
		deps []string
	}{
		{mods.StageNamePlan, mods.StageNamePlan, "node-wasm", nil},
		{mods.StageNameORWApply, mods.StageNameORWApply, "node-wasm", []string{mods.StageNamePlan}},
		{mods.StageNameORWGenerate, mods.StageNameORWGenerate, "node-wasm", []string{mods.StageNamePlan}},
		{mods.StageNameLLMPlan, mods.StageNameLLMPlan, "gpu-ml", []string{mods.StageNamePlan}},
		{mods.StageNameLLMExec, mods.StageNameLLMExec, "gpu-ml", []string{mods.StageNameORWApply, mods.StageNameORWGenerate, mods.StageNameLLMPlan}},
		{mods.StageNameHuman, mods.StageNameHuman, "go-native", []string{mods.StageNameLLMExec}},
	}
	for i, exp := range expect {
		stage := stages[i]
		if stage.Name != exp.name {
			t.Fatalf("unexpected stage %d name: got %s want %s", i, stage.Name, exp.name)
		}
		if stage.Kind != exp.kind {
			t.Fatalf("unexpected stage %s kind: got %s want %s", stage.Name, stage.Kind, exp.kind)
		}
		if stage.Lane != exp.lane {
			t.Fatalf("unexpected stage %s lane: got %s want %s", stage.Name, stage.Lane, exp.lane)
		}
		if len(stage.Dependencies) != len(exp.deps) {
			t.Fatalf("unexpected dependency count for %s: got %d want %d", stage.Name, len(stage.Dependencies), len(exp.deps))
		}
		for j, dep := range exp.deps {
			if stage.Dependencies[j] != dep {
				t.Fatalf("unexpected dependency %d for %s: got %s want %s", j, stage.Name, stage.Dependencies[j], dep)
			}
		}
	}
}
