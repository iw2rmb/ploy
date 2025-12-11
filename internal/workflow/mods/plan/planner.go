package plan

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Options configures the Mods planner advisor and execution hints.
type Options struct {
	Advisor     Advisor
	PlanTimeout time.Duration
	MaxParallel int
}

// PlanInput carries workflow run context for planner evaluation.
type PlanInput struct {
	Run     contracts.WorkflowRun
	Signals AdviceSignals
}

// Planner constructs Mods workflow stages.
type Planner struct {
	opts Options
}

// NewPlanner builds a Planner with the provided options.
func NewPlanner(opts Options) Planner {
	return Planner{opts: applyDefaults(opts)}
}

// Plan assembles the Mods stage graph for the given run envelope.
func (p Planner) Plan(ctx context.Context, in PlanInput) ([]Stage, error) {
	plan := []Stage{
		{Name: StageNamePlan, Kind: StageKindPlan},
		{Name: StageNameORWApply, Kind: StageKindORWApply, Dependencies: []string{StageNamePlan}},
		{Name: StageNameORWGenerate, Kind: StageKindORWGenerate, Dependencies: []string{StageNamePlan}},
		// Human review depends on both ORW apply and generate stages completing.
		{Name: StageNameHuman, Kind: StageKindHuman, Dependencies: []string{StageNameORWApply, StageNameORWGenerate}},
	}

	p.applyAdvisor(ctx, plan, in.Run, in.Signals)
	p.applyPlannerHints(plan)
	return plan, nil
}
