package plan

import (
	"context"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Options configures the Mods planner output lanes and advisor.
type Options struct {
	PlanLane        string
	OpenRewriteLane string
	LLMPlanLane     string
	LLMExecLane     string
	HumanLane       string
	Advisor         Advisor
	PlanTimeout     time.Duration
	MaxParallel     int
}

// PlanInput carries ticket context for planner evaluation.
type PlanInput struct {
	Ticket  contracts.WorkflowTicket
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

// Plan assembles the Mods stage graph for the given ticket.
func (p Planner) Plan(ctx context.Context, in PlanInput) ([]Stage, error) {
	plan := []Stage{
		{Name: StageNamePlan, Kind: StageKindPlan, Lane: p.opts.PlanLane},
		{Name: StageNameORWApply, Kind: StageKindORWApply, Lane: p.opts.OpenRewriteLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameORWGenerate, Kind: StageKindORWGenerate, Lane: p.opts.OpenRewriteLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameLLMPlan, Kind: StageKindLLMPlan, Lane: p.opts.LLMPlanLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameLLMExec, Kind: StageKindLLMExec, Lane: p.opts.LLMExecLane, Dependencies: []string{StageNameORWApply, StageNameORWGenerate, StageNameLLMPlan}},
		{Name: StageNameHuman, Kind: StageKindHuman, Lane: p.opts.HumanLane, Dependencies: []string{StageNameLLMExec}},
	}

	p.applyAdvisor(ctx, plan, in.Ticket, in.Signals)
	p.applyPlannerHints(plan)

	for i := range plan {
		plan[i].Lane = strings.TrimSpace(plan[i].Lane)
	}
	return plan, nil
}
