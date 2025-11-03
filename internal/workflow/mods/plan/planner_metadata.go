package plan

import (
	"context"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

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

// applyPlannerHints mounts planner hints such as timeout and max parallel hints.
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
