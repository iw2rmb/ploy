package mods

import (
	"context"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	StageNamePlan        = "mods-plan"
	StageNameORWApply    = "orw-apply"
	StageNameORWGenerate = "orw-gen"
	StageNameLLMPlan     = "llm-plan"
	StageNameLLMExec     = "llm-exec"
	StageNameHuman       = "mods-human"

	StageKindPlan        = StageNamePlan
	StageKindORWApply    = StageNameORWApply
	StageKindORWGenerate = StageNameORWGenerate
	StageKindLLMPlan     = StageNameLLMPlan
	StageKindLLMExec     = StageNameLLMExec
	StageKindHuman       = StageNameHuman

	defaultPlanLane        = "node-wasm"
	defaultOpenRewriteLane = "node-wasm"
	defaultLLMPlanLane     = "gpu-ml"
	defaultLLMExecLane     = "gpu-ml"
	defaultHumanLane       = "go-native"
)

// Stage models a Mods workflow stage produced by the planner.
type Stage struct {
	Name         string
	Kind         string
	Lane         string
	Dependencies []string
}

// Options configures the Mods planner output lanes.
type Options struct {
	PlanLane        string
	OpenRewriteLane string
	LLMPlanLane     string
	LLMExecLane     string
	HumanLane       string
}

// PlanInput carries ticket context for planner evaluation.
type PlanInput struct {
	Ticket contracts.WorkflowTicket
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
	_ = ctx
	_ = in

	plan := []Stage{
		{Name: StageNamePlan, Kind: StageKindPlan, Lane: p.opts.PlanLane},
		{Name: StageNameORWApply, Kind: StageKindORWApply, Lane: p.opts.OpenRewriteLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameORWGenerate, Kind: StageKindORWGenerate, Lane: p.opts.OpenRewriteLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameLLMPlan, Kind: StageKindLLMPlan, Lane: p.opts.LLMPlanLane, Dependencies: []string{StageNamePlan}},
		{Name: StageNameLLMExec, Kind: StageKindLLMExec, Lane: p.opts.LLMExecLane, Dependencies: []string{StageNameORWApply, StageNameORWGenerate, StageNameLLMPlan}},
		{Name: StageNameHuman, Kind: StageKindHuman, Lane: p.opts.HumanLane, Dependencies: []string{StageNameLLMExec}},
	}

	for i := range plan {
		plan[i].Lane = strings.TrimSpace(plan[i].Lane)
	}
	return plan, nil
}

func applyDefaults(opts Options) Options {
	result := opts
	if strings.TrimSpace(result.PlanLane) == "" {
		result.PlanLane = defaultPlanLane
	}
	if strings.TrimSpace(result.OpenRewriteLane) == "" {
		result.OpenRewriteLane = defaultOpenRewriteLane
	}
	if strings.TrimSpace(result.LLMPlanLane) == "" {
		result.LLMPlanLane = defaultLLMPlanLane
	}
	if strings.TrimSpace(result.LLMExecLane) == "" {
		result.LLMExecLane = defaultLLMExecLane
	}
	if strings.TrimSpace(result.HumanLane) == "" {
		result.HumanLane = defaultHumanLane
	}
	return result
}
