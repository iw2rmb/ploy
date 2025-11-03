package mods

import (
	"github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

const (
	// StageNamePlan re-exports plan.StageNamePlan for legacy consumers.
	StageNamePlan = plan.StageNamePlan
	// StageNameORWApply re-exports plan.StageNameORWApply for legacy consumers.
	StageNameORWApply = plan.StageNameORWApply
	// StageNameORWGenerate re-exports plan.StageNameORWGenerate for legacy consumers.
	StageNameORWGenerate = plan.StageNameORWGenerate
	// StageNameLLMPlan re-exports plan.StageNameLLMPlan for legacy consumers.
	StageNameLLMPlan = plan.StageNameLLMPlan
	// StageNameLLMExec re-exports plan.StageNameLLMExec for legacy consumers.
	StageNameLLMExec = plan.StageNameLLMExec
	// StageNameHuman re-exports plan.StageNameHuman for legacy consumers.
	StageNameHuman = plan.StageNameHuman

	// StageKindPlan re-exports plan.StageKindPlan for legacy consumers.
	StageKindPlan = plan.StageKindPlan
	// StageKindORWApply re-exports plan.StageKindORWApply for legacy consumers.
	StageKindORWApply = plan.StageKindORWApply
	// StageKindORWGenerate re-exports plan.StageKindORWGenerate for legacy consumers.
	StageKindORWGenerate = plan.StageKindORWGenerate
	// StageKindLLMPlan re-exports plan.StageKindLLMPlan for legacy consumers.
	StageKindLLMPlan = plan.StageKindLLMPlan
	// StageKindLLMExec re-exports plan.StageKindLLMExec for legacy consumers.
	StageKindLLMExec = plan.StageKindLLMExec
	// StageKindHuman re-exports plan.StageKindHuman for legacy consumers.
	StageKindHuman = plan.StageKindHuman
)

// Options re-exports planner options for compatibility.
type Options = plan.Options

// PlanInput re-exports planner input for compatibility.
type PlanInput = plan.PlanInput

// Advisor re-exports the knowledge-base advisor interface.
type Advisor = plan.Advisor

// AdviceRequest re-exports advisor request payload.
type AdviceRequest = plan.AdviceRequest

// Advice re-exports advisor response payload.
type Advice = plan.Advice

// AdvicePlan re-exports plan.AdvicePlan.
type AdvicePlan = plan.AdvicePlan

// AdviceHuman re-exports plan.AdviceHuman.
type AdviceHuman = plan.AdviceHuman

// AdviceRecommendation re-exports plan.AdviceRecommendation.
type AdviceRecommendation = plan.AdviceRecommendation

// AdviceSignals re-exports plan.AdviceSignals.
type AdviceSignals = plan.AdviceSignals

// Stage re-exports plan.Stage for compatibility.
type Stage = plan.Stage

// StageMetadata re-exports plan.StageMetadata for compatibility.
type StageMetadata = plan.StageMetadata

// StageModsMetadata re-exports plan.StageModsMetadata for compatibility.
type StageModsMetadata = plan.StageModsMetadata

// StageModsPlan re-exports plan.StageModsPlan for compatibility.
type StageModsPlan = plan.StageModsPlan

// StageModsHuman re-exports plan.StageModsHuman for compatibility.
type StageModsHuman = plan.StageModsHuman

// StageModsRecommendation re-exports plan.StageModsRecommendation for compatibility.
type StageModsRecommendation = plan.StageModsRecommendation

// Planner re-exports plan.Planner for compatibility.
type Planner = plan.Planner

// NewPlanner constructs a planner using the shared plan package.
func NewPlanner(opts Options) Planner {
	return plan.NewPlanner(opts)
}
