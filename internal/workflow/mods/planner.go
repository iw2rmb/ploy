package mods

import (
	"context"
	"strings"
	"time"

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
	Metadata     StageMetadata
}

// StageMetadata holds Mods-specific metadata for a workflow stage.
type StageMetadata struct {
	Mods *StageModsMetadata
}

// StageModsMetadata captures Mods plan, human, and recommendation payloads.
type StageModsMetadata struct {
	Plan            *StageModsPlan
	Human           *StageModsHuman
	Recommendations []StageModsRecommendation
}

// StageModsPlan describes the Mods planner output that Grid consumers rely on.
type StageModsPlan struct {
	SelectedRecipes []string
	ParallelStages  []string
	HumanGate       bool
	Summary         string
	PlanTimeout     string
	MaxParallel     int
}

// StageModsHuman outlines expectations for the human-in-the-loop checkpoint.
type StageModsHuman struct {
	Required  bool
	Playbooks []string
}

// StageModsRecommendation records individual recommendations surfaced by the
// Mods advisor.
type StageModsRecommendation struct {
	Source     string
	Message    string
	Confidence float64
}

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
	if result.PlanTimeout < 0 {
		result.PlanTimeout = 0
	}
	if result.MaxParallel < 0 {
		result.MaxParallel = 0
	}
	return result
}

// applyAdvisor enriches the Mods stages with advisor metadata when available.
func (p Planner) applyAdvisor(ctx context.Context, stages []Stage, ticket contracts.WorkflowTicket, signals AdviceSignals) {
	advisor := p.opts.Advisor
	if advisor == nil {
		return
	}
	advice, err := advisor.Advise(ctx, AdviceRequest{Ticket: ticket, Signals: signals})
	if err != nil {
		return
	}
	planStage := stageByName(stages, StageNamePlan)
	if planStage != nil {
		modsMeta := ensureModsMetadata(planStage)
		if plan := buildStagePlanMetadata(advice.Plan); plan != nil {
			modsMeta.Plan = plan
		}
		if recs := buildStageRecommendations(advice.Recommendations); len(recs) > 0 {
			modsMeta.Recommendations = recs
		}
		if modsMeta.Plan == nil && len(modsMeta.Recommendations) == 0 {
			planStage.Metadata.Mods = nil
		}
	}
	humanStage := stageByName(stages, StageNameHuman)
	if humanStage != nil {
		modsMeta := ensureModsMetadata(humanStage)
		if human := buildStageHumanMetadata(advice.Human); human != nil {
			modsMeta.Human = human
		}
		if modsMeta.Human == nil {
			humanStage.Metadata.Mods = nil
		}
	}
}

func (p Planner) applyPlannerHints(stages []Stage) {
	planStage := stageByName(stages, StageNamePlan)
	if planStage == nil {
		return
	}
	timeout := p.opts.PlanTimeout
	if timeout < 0 {
		timeout = 0
	}
	maxParallel := p.opts.MaxParallel
	if maxParallel < 0 {
		maxParallel = 0
	}
	if timeout <= 0 && maxParallel <= 0 {
		return
	}
	modsMeta := ensureModsMetadata(planStage)
	if modsMeta.Plan == nil {
		modsMeta.Plan = &StageModsPlan{}
	}
	if timeout > 0 {
		modsMeta.Plan.PlanTimeout = formatPlanTimeout(timeout)
	}
	if maxParallel > 0 {
		modsMeta.Plan.MaxParallel = maxParallel
	}
	if len(modsMeta.Plan.ParallelStages) == 0 {
		modsMeta.Plan.ParallelStages = []string{StageNameORWApply, StageNameORWGenerate}
	}
}

func formatPlanTimeout(timeout time.Duration) string {
	if timeout <= 0 {
		return ""
	}
	if timeout%time.Millisecond == 0 {
		return timeout.String()
	}
	trimmed := timeout.Truncate(time.Millisecond)
	if trimmed <= 0 {
		trimmed = time.Millisecond
	}
	return trimmed.String()
}

// stageByName returns the pointer to the stage matching the provided name.
func stageByName(stages []Stage, name string) *Stage {
	for i := range stages {
		if stages[i].Name == name {
			return &stages[i]
		}
	}
	return nil
}

// ensureModsMetadata initialises Mods metadata on a stage when needed.
func ensureModsMetadata(stage *Stage) *StageModsMetadata {
	if stage.Metadata.Mods == nil {
		stage.Metadata.Mods = &StageModsMetadata{}
	}
	return stage.Metadata.Mods
}

// buildStagePlanMetadata converts advisor plan guidance into stage metadata.
func buildStagePlanMetadata(advice AdvicePlan) *StageModsPlan {
	recipes := copyStrings(advice.SelectedRecipes)
	parallel := copyStrings(advice.ParallelStages)
	summary := strings.TrimSpace(advice.Summary)
	if len(parallel) == 0 {
		parallel = []string{StageNameORWApply, StageNameORWGenerate}
	}
	if len(recipes) == 0 && !advice.HumanGate && summary == "" {
		return nil
	}
	return &StageModsPlan{
		SelectedRecipes: recipes,
		ParallelStages:  parallel,
		HumanGate:       advice.HumanGate,
		Summary:         summary,
	}
}

// buildStageHumanMetadata maps advisor human guidance into stage metadata.
func buildStageHumanMetadata(advice AdviceHuman) *StageModsHuman {
	playbooks := copyStrings(advice.Playbooks)
	if !advice.Required && len(playbooks) == 0 {
		return nil
	}
	return &StageModsHuman{
		Required:  advice.Required,
		Playbooks: playbooks,
	}
}

// buildStageRecommendations normalises advisor recommendations.
func buildStageRecommendations(values []AdviceRecommendation) []StageModsRecommendation {
	if len(values) == 0 {
		return nil
	}
	result := make([]StageModsRecommendation, 0, len(values))
	for _, value := range values {
		message := strings.TrimSpace(value.Message)
		if message == "" {
			continue
		}
		source := strings.TrimSpace(value.Source)
		confidence := value.Confidence
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		result = append(result, StageModsRecommendation{
			Source:     source,
			Message:    message,
			Confidence: confidence,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// copyStrings trims and copies non-empty string values.
func copyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}
