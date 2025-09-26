package mods_test

import (
	"context"
	"errors"
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

// TestPlannerAnnotatesModsMetadataFromAdvisor verifies that metadata from the
// knowledge base advisor is propagated into the Mods planning stages.
func TestPlannerAnnotatesModsMetadataFromAdvisor(t *testing.T) {
	advisor := &stubAdvisor{
		advice: mods.Advice{
			Plan: mods.AdvicePlan{
				SelectedRecipes: []string{"recipe.alpha", "recipe.beta"},
				ParallelStages:  []string{mods.StageNameORWApply, mods.StageNameORWGenerate},
				HumanGate:       true,
				Summary:         "expect human review after LLM execution",
			},
			Human: mods.AdviceHuman{
				Required:  true,
				Playbooks: []string{"playbook.mods.review"},
			},
			Recommendations: []mods.AdviceRecommendation{
				{Source: "knowledge-base", Message: "Apply recipe.alpha first", Confidence: 0.91},
				{Source: "knowledge-base", Message: "Queue review if diff size > 500", Confidence: 1.4},
				{Source: "knowledge-base", Message: " ", Confidence: -0.5},
			},
		},
	}

	planner := mods.NewPlanner(mods.Options{Advisor: advisor})

	stages, err := planner.Plan(context.Background(), mods.PlanInput{
		Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-knowledge", Tenant: "acme"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if !advisor.called {
		t.Fatalf("expected planner to invoke advisor")
	}

	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods == nil || planStage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected mods plan metadata present, got %#v", planStage.Metadata)
	}
	planMeta := planStage.Metadata.Mods.Plan
	if planMeta.HumanGate != true {
		t.Fatalf("expected human gate suggestion true, got %v", planMeta.HumanGate)
	}
	if len(planMeta.SelectedRecipes) != 2 {
		t.Fatalf("expected selected recipes recorded, got %#v", planMeta.SelectedRecipes)
	}
	if len(planMeta.ParallelStages) != 2 {
		t.Fatalf("expected parallel stages recorded, got %#v", planMeta.ParallelStages)
	}
	if planMeta.Summary == "" {
		t.Fatalf("expected summary present")
	}
	if len(planStage.Metadata.Mods.Recommendations) != 2 {
		t.Fatalf("expected recommendations recorded, got %#v", planStage.Metadata.Mods.Recommendations)
	}
	recs := planStage.Metadata.Mods.Recommendations
	if recs[1].Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %#v", recs[1].Confidence)
	}

	humanStage := findStage(t, stages, mods.StageNameHuman)
	if humanStage.Metadata.Mods == nil || humanStage.Metadata.Mods.Human == nil {
		t.Fatalf("expected human metadata present, got %#v", humanStage.Metadata)
	}
	humanMeta := humanStage.Metadata.Mods.Human
	if !humanMeta.Required {
		t.Fatalf("expected human stage required")
	}
	if len(humanMeta.Playbooks) != 1 {
		t.Fatalf("expected human playbook recorded, got %#v", humanMeta.Playbooks)
	}
}

// TestPlannerFallsBackWhenAdvisorErrors ensures planner metadata remains stable
// when the advisor fails.
func TestPlannerFallsBackWhenAdvisorErrors(t *testing.T) {
	errAdvisor := &stubAdvisor{err: errors.New("advisor unavailable")}
	planner := mods.NewPlanner(mods.Options{Advisor: errAdvisor})

	stages, err := planner.Plan(context.Background(), mods.PlanInput{
		Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-fallback", Tenant: "acme"},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if !errAdvisor.called {
		t.Fatalf("expected planner to invoke advisor even when it errors")
	}

	planStage := findStage(t, stages, mods.StageNamePlan)
	if planStage.Metadata.Mods != nil {
		t.Fatalf("expected no mods metadata on failure, got %#v", planStage.Metadata.Mods)
	}
}

// findStage locates a stage by name inside the Mods planner output during tests.
func findStage(t *testing.T, stages []mods.Stage, name string) mods.Stage {
	t.Helper()
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}
	t.Fatalf("stage %s not found", name)
	return mods.Stage{}
}

// stubAdvisor implements mods.Advisor for tests.
type stubAdvisor struct {
	advice mods.Advice
	err    error
	called bool
}

// Advise returns the stubbed advisor response for tests.
func (s *stubAdvisor) Advise(ctx context.Context, req mods.AdviceRequest) (mods.Advice, error) {
	s.called = true
	if s.err != nil {
		return mods.Advice{}, s.err
	}
	return s.advice, nil
}
