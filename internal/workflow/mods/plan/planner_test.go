package plan

import (
	"context"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type stubAdvisor struct {
	advice Advice
	err    error
}

func (s stubAdvisor) Advise(ctx context.Context, req AdviceRequest) (Advice, error) {
	return s.advice, s.err
}

func TestPlannerPlanWithDefaultsAndAdvisor(t *testing.T) {
	adv := Advice{
		Plan:            AdvicePlan{SelectedRecipes: []string{"a"}, Summary: "ok"},
		Human:           AdviceHuman{Required: true, Playbooks: []string{"p"}},
		Recommendations: []AdviceRecommendation{{Source: "kb", Message: "do x", Confidence: 1.2}},
	}
	p := NewPlanner(Options{Advisor: stubAdvisor{advice: adv}, PlanTimeout: 1500 * time.Millisecond, MaxParallel: 3})
	stages, err := p.Plan(context.Background(), PlanInput{Run: contracts.WorkflowRun{SchemaVersion: contracts.SchemaVersion, RunID: types.RunID("run-test")}})
	if err != nil {
		t.Fatalf("Plan err=%v", err)
	}
	if len(stages) == 0 {
		t.Fatalf("expected stages")
	}
	// Check plan and human stages metadata were populated.
	plan := stageByName(stages, StageNamePlan)
	if plan == nil || plan.Metadata.Mods == nil || plan.Metadata.Mods.Plan == nil || plan.Metadata.Mods.Plan.PlanTimeout == "" || plan.Metadata.Mods.Plan.MaxParallel != 3 {
		t.Fatalf("plan metadata not set: %+v", plan)
	}
	human := stageByName(stages, StageNameHuman)
	if human == nil || human.Metadata.Mods == nil || human.Metadata.Mods.Human == nil || !human.Metadata.Mods.Human.Required {
		t.Fatalf("human metadata not set: %+v", human)
	}
	if len(plan.Metadata.Mods.Recommendations) == 0 {
		t.Fatalf("recommendations missing")
	}
}

func TestFormatPlanTimeoutAndCopyStrings(t *testing.T) {
	if got := formatPlanTimeout(0); got != "" {
		t.Fatalf("formatPlanTimeout(0)=%q", got)
	}
	if got := formatPlanTimeout(1500*time.Millisecond + 200*time.Microsecond); got != "1.5s" {
		t.Fatalf("formatPlanTimeout rounding=%q", got)
	}
	if out := copyStrings([]string{" a ", "", "b"}); len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("copyStrings=%v", out)
	}
}
