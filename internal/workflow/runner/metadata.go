package runner

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

// convertStageMetadata maps Mods planner metadata onto runner stage metadata.
func convertStageMetadata(meta mods.StageMetadata) StageMetadata {
	result := StageMetadata{}
	if converted := convertModsMetadata(meta.Mods); converted != nil {
		result.Mods = converted
	}
	return result
}

// convertModsMetadata clones Mods metadata from the planner onto runner stages.
func convertModsMetadata(src *mods.StageModsMetadata) *StageModsMetadata {
	if src == nil {
		return nil
	}
	dst := &StageModsMetadata{}
	if src.Plan != nil {
		dst.Plan = &StageModsPlan{
			SelectedRecipes: copyStringSlice(src.Plan.SelectedRecipes),
			ParallelStages:  copyStringSlice(src.Plan.ParallelStages),
			HumanGate:       src.Plan.HumanGate,
			Summary:         strings.TrimSpace(src.Plan.Summary),
			PlanTimeout:     strings.TrimSpace(src.Plan.PlanTimeout),
			MaxParallel:     src.Plan.MaxParallel,
		}
	}
	if src.Human != nil {
		dst.Human = &StageModsHuman{
			Required:  src.Human.Required,
			Playbooks: copyStringSlice(src.Human.Playbooks),
		}
	}
	if len(src.Recommendations) > 0 {
		dst.Recommendations = make([]StageModsRecommendation, 0, len(src.Recommendations))
		for _, rec := range src.Recommendations {
			source := strings.TrimSpace(rec.Source)
			message := strings.TrimSpace(rec.Message)
			if message == "" {
				continue
			}
			dst.Recommendations = append(dst.Recommendations, StageModsRecommendation{
				Source:     source,
				Message:    message,
				Confidence: clampConfidence(rec.Confidence),
			})
		}
	}
	if dst.Plan == nil && dst.Human == nil && len(dst.Recommendations) == 0 {
		return nil
	}
	return dst
}

// clampConfidence constrains confidence scores to the [0,1] range.
func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
