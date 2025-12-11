package plan_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	plan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

func TestPlannerBuildsModsStageGraph_Integration(t *testing.T) {
	planner := plan.NewPlanner(plan.Options{})
	stages, err := planner.Plan(context.Background(), plan.PlanInput{
		Run: contracts.WorkflowRun{SchemaVersion: contracts.SchemaVersion, RunID: types.RunID("run-123")},
	})
	if err != nil {
		t.Fatalf("unexpected error planning mods stages: %v", err)
	}
	if len(stages) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(stages))
	}
	expect := []struct {
		name, kind string
		deps       []string
	}{
		{plan.StageNamePlan, plan.StageKindPlan, nil},
		{plan.StageNameORWApply, plan.StageKindORWApply, []string{plan.StageNamePlan}},
		{plan.StageNameORWGenerate, plan.StageKindORWGenerate, []string{plan.StageNamePlan}},
		{plan.StageNameHuman, plan.StageKindHuman, []string{plan.StageNameORWApply, plan.StageNameORWGenerate}},
	}
	for i, exp := range expect {
		s := stages[i]
		if s.Name != exp.name || s.Kind != exp.kind || !reflect.DeepEqual(s.Dependencies, exp.deps) {
			t.Fatalf("unexpected stage[%d]: %+v", i, s)
		}
	}
}

func TestPlannerAnnotatesMetadataFromAdvisor(t *testing.T) {
	advisor := &stubAdvisor{advice: plan.Advice{
		Plan:            plan.AdvicePlan{SelectedRecipes: []string{"recipe.alpha", "recipe.beta"}, ParallelStages: []string{plan.StageNameORWApply, plan.StageNameORWGenerate}, HumanGate: true, Summary: "expect human review after LLM execution"},
		Human:           plan.AdviceHuman{Required: true, Playbooks: []string{"playbook.mods.review"}},
		Recommendations: []plan.AdviceRecommendation{{Source: "advisor", Message: "Apply recipe.alpha first", Confidence: 0.91}, {Source: "advisor", Message: "Queue review if diff size > 500", Confidence: 1.4}, {Source: "advisor", Message: " ", Confidence: -0.5}},
	}}
	p := plan.NewPlanner(plan.Options{Advisor: advisor})
	stages, err := p.Plan(context.Background(), plan.PlanInput{Run: contracts.WorkflowRun{SchemaVersion: contracts.SchemaVersion, RunID: types.RunID("run-knowledge")}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !advisor.called {
		t.Fatalf("advisor not called")
	}
	planStage := findStage(t, stages, plan.StageNamePlan)
	if planStage.Metadata.Mods == nil || planStage.Metadata.Mods.Plan == nil || len(planStage.Metadata.Mods.Recommendations) != 2 {
		t.Fatalf("missing plan metadata: %#v", planStage.Metadata)
	}
	humanStage := findStage(t, stages, plan.StageNameHuman)
	if humanStage.Metadata.Mods == nil || humanStage.Metadata.Mods.Human == nil || !humanStage.Metadata.Mods.Human.Required {
		t.Fatalf("missing human metadata")
	}
}

func TestPlannerFallsBackWhenAdvisorErrors(t *testing.T) {
	errAdv := &stubAdvisor{err: errors.New("advisor unavailable")}
	p := plan.NewPlanner(plan.Options{Advisor: errAdv})
	stages, err := p.Plan(context.Background(), plan.PlanInput{Run: contracts.WorkflowRun{SchemaVersion: contracts.SchemaVersion, RunID: types.RunID("run-fallback")}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !errAdv.called {
		t.Fatalf("advisor not called")
	}
	planStage := findStage(t, stages, plan.StageNamePlan)
	if planStage.Metadata.Mods != nil {
		t.Fatalf("expected no mods metadata on failure, got %#v", planStage.Metadata.Mods)
	}
}

func TestPlannerExposesExecutionHints(t *testing.T) {
	opts := plan.Options{}
	setOptionsHints(t, &opts, 90*time.Second, 3)
	p := plan.NewPlanner(opts)
	stages, err := p.Plan(context.Background(), plan.PlanInput{Run: contracts.WorkflowRun{SchemaVersion: contracts.SchemaVersion, RunID: types.RunID("run-hints")}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	planStage := findStage(t, stages, plan.StageNamePlan)
	if planStage.Metadata.Mods == nil || planStage.Metadata.Mods.Plan == nil {
		t.Fatalf("expected plan metadata present")
	}
	pm := *planStage.Metadata.Mods.Plan
	if len(pm.ParallelStages) == 0 || pm.PlanTimeout != "1m30s" || pm.MaxParallel != 3 {
		t.Fatalf("unexpected plan meta: %#v", pm)
	}
}

func TestPlannerIntegratesKnowledgeBaseAdvisor(t *testing.T) {
	advisor := &stubAdvisor{advice: plan.Advice{
		Plan: plan.AdvicePlan{
			SelectedRecipes: []string{"recipe.npm.lint"},
			HumanGate:       true,
			Summary:         "Run npm run lint with fixes",
		},
		Human: plan.AdviceHuman{
			Required:  true,
			Playbooks: []string{"mods.npm.lint"},
		},
	}}
	p := plan.NewPlanner(plan.Options{Advisor: advisor})
	stages, err := p.Plan(context.Background(), plan.PlanInput{
		Run: contracts.WorkflowRun{
			SchemaVersion: contracts.SchemaVersion,
			RunID:         types.RunID("KB-1"),
			Manifest:      contracts.ManifestReference{Name: "repo", Version: "1.0.0"},
		},
		Signals: plan.AdviceSignals{Errors: []string{"npm ERR! lint script failed"}},
	})
	if err != nil {
		t.Fatalf("plan kb: %v", err)
	}
	ps := findStage(t, stages, plan.StageNamePlan)
	if ps.Metadata.Mods == nil || ps.Metadata.Mods.Plan == nil || len(ps.Metadata.Mods.Plan.SelectedRecipes) == 0 || !ps.Metadata.Mods.Plan.HumanGate {
		t.Fatalf("plan metadata: %#v", ps.Metadata)
	}
	hs := findStage(t, stages, plan.StageNameHuman)
	if hs.Metadata.Mods == nil || hs.Metadata.Mods.Human == nil || len(hs.Metadata.Mods.Human.Playbooks) == 0 {
		t.Fatalf("human metadata: %#v", hs.Metadata)
	}
}

func setOptionsHints(t *testing.T, opts *plan.Options, planTimeout time.Duration, maxParallel int) {
	t.Helper()
	v := reflect.ValueOf(opts).Elem()
	if f := v.FieldByName("PlanTimeout"); f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(planTimeout))
	}
	if f := v.FieldByName("MaxParallel"); f.IsValid() && f.CanSet() {
		f.SetInt(int64(maxParallel))
	}
}

func findStage(t *testing.T, stages []plan.Stage, name string) plan.Stage {
	t.Helper()
	for _, s := range stages {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("stage %s not found", name)
	return plan.Stage{}
}

type stubAdvisor struct {
	advice plan.Advice
	err    error
	called bool
}

func (s *stubAdvisor) Advise(ctx context.Context, req plan.AdviceRequest) (plan.Advice, error) {
	s.called = true
	if s.err != nil {
		return plan.Advice{}, s.err
	}
	return s.advice, nil
}
