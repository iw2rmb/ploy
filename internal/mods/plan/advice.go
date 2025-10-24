package plan

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Advisor exposes Mods knowledge base guidance to the planner.
type Advisor interface {
	Advise(ctx context.Context, req AdviceRequest) (Advice, error)
}

// AdviceRequest wraps the workflow ticket passed to the advisor.
type AdviceRequest struct {
	Ticket  contracts.WorkflowTicket
	Signals AdviceSignals
}

// Advice aggregates planner hints, human expectations, and recommendations.
type Advice struct {
	Plan            AdvicePlan
	Human           AdviceHuman
	Recommendations []AdviceRecommendation
}

// AdviceSignals captures contextual signals gathered for classification.
type AdviceSignals struct {
	Errors   []string
	Manifest contracts.ManifestReference
}

// AdvicePlan represents the Mods planner advice returned by the knowledge base.
type AdvicePlan struct {
	SelectedRecipes []string
	ParallelStages  []string
	HumanGate       bool
	Summary         string
}

// AdviceHuman captures human-stage cues returned by the knowledge base.
type AdviceHuman struct {
	Required  bool
	Playbooks []string
}

// AdviceRecommendation captures a single knowledge base recommendation.
type AdviceRecommendation struct {
	Source     string
	Message    string
	Confidence float64
}
