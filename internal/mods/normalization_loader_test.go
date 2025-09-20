package mods

import (
	"context"
	"testing"
	"time"
)

// fakeJobHelper returns a plan with a human alias and a stop next action.
type fakeJobHelper struct{}

func (f *fakeJobHelper) SubmitPlannerJob(ctx context.Context, config *ModConfig, buildError string, workspace string) (*PlanResult, error) {
	return &PlanResult{PlanID: "p-1", Options: []map[string]any{{"id": "opt1", "type": "human"}}}, nil
}
func (f *fakeJobHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	return &NextAction{Action: "stop", Notes: "ok"}, nil
}

// captureHealer captures branches passed to fanout and returns a quick success.
type captureHealer struct{ branches []BranchSpec }

func (c *captureHealer) RunFanout(ctx context.Context, runCtx any, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	c.branches = append([]BranchSpec(nil), branches...)
	// fabricate results aligned with branches
	results := make([]BranchResult, len(branches))
	for i, b := range branches {
		results[i] = BranchResult{ID: b.ID, Status: "completed", StartedAt: time.Now(), FinishedAt: time.Now()}
	}
	return results[0], results, nil
}

func TestPlannerHumanAliasIsNormalizedBeforeFanout(t *testing.T) {
	cfg := &ModConfig{ID: "w1", TargetRepo: "https://x/y.git", BaseRef: "main", Steps: []ModStep{{Type: "recipe", ID: "s"}}, SelfHeal: GetDefaultSelfHealConfig()}
	r, err := NewModRunner(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	fh := &fakeJobHelper{}
	ch := &captureHealer{}
	r.SetJobHelper(fh)
	r.SetHealingOrchestrator(ch)

	_, err = r.attemptHealing(context.Background(), "/dev/null", "build failed", nil)
	if err != nil {
		t.Fatalf("attemptHealing err: %v", err)
	}

	if len(ch.branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(ch.branches))
	}
	if got := ch.branches[0].Type; got != string(StepTypeHumanStep) {
		t.Fatalf("expected normalized branch type %q, got %q", StepTypeHumanStep, got)
	}
}
