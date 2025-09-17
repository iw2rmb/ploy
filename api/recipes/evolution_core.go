package recipes

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/api/recipes/models"
)

// NewRecipeEvolution creates a new recipe evolution system
func NewRecipeEvolution(registry *RecipeRegistry, validator RecipeValidator, versioning RecipeVersioning) RecipeEvolution {
	config := RecipeEvolutionConfig{
		MaxEvolutionDepth:     5,
		MinConfidenceRequired: 0.7,
		EnableAutoApproval:    false,
		AutoApprovalThreshold: 0.9,
		RetainVersionHistory:  10,
	}

	return &DefaultRecipeEvolution{
		registry:   registry,
		validator:  validator,
		versioning: versioning,
		config:     config,
	}
}

// EvolveRecipe applies the suggested modifications to create an evolved recipe
func (re *DefaultRecipeEvolution) EvolveRecipe(ctx context.Context, recipe *models.Recipe, analysis FailureAnalysis) (*models.Recipe, error) {
	if analysis.Confidence < re.config.MinConfidenceRequired {
		return nil, fmt.Errorf("analysis confidence %.2f below required threshold %.2f",
			analysis.Confidence, re.config.MinConfidenceRequired)
	}

	// Create a copy of the original recipe
	evolved := recipe
	evolved.Metadata.Version = recipe.Metadata.Version + ".evolved"
	evolved.Metadata.Description += " (auto-evolved based on failure analysis)"

	// Apply modifications based on priority
	modifications := analysis.SuggestedFixes
	for i := 0; i < len(modifications); i++ {
		for j := i + 1; j < len(modifications); j++ {
			if modifications[i].Priority > modifications[j].Priority {
				modifications[i], modifications[j] = modifications[j], modifications[i]
			}
		}
	}

	// Apply each modification
	evolutionNotes := []string{}
	for _, mod := range modifications {
		if mod.RiskLevel == RiskLevelHigh && !re.config.EnableAutoApproval {
			// Skip high-risk modifications without approval
			continue
		}

		switch mod.Type {
		case ModificationAddRule:
			evolved = re.applyAddRule(evolved, mod)
		case ModificationModifyRule:
			evolved = re.applyModifyRule(evolved, mod)
		case ModificationAddCondition:
			evolved = re.applyAddCondition(evolved, mod)
		case ModificationAddException:
			evolved = re.applyAddException(evolved, mod)
		case ModificationAdjustPattern:
			evolved = re.applyAdjustPattern(evolved, mod)
		case ModificationExtendScope:
			evolved = re.applyExtendScope(evolved, mod)
		case ModificationReduceScope:
			evolved = re.applyReduceScope(evolved, mod)
		}

		evolutionNotes = append(evolutionNotes, fmt.Sprintf("%s: %s", mod.Type, mod.Justification))
	}

	// Update recipe metadata with evolution information
	evolved.Metadata.Tags = append(evolved.Metadata.Tags, "evolved")
	evolved.Metadata.Tags = append(evolved.Metadata.Tags, fmt.Sprintf("evolved-from:%s", recipe.ID))
	evolved.Metadata.Tags = append(evolved.Metadata.Tags, fmt.Sprintf("confidence:%.3f", analysis.Confidence))

	// Add evolution notes as tags
	for _, note := range evolutionNotes {
		evolved.Metadata.Tags = append(evolved.Metadata.Tags, fmt.Sprintf("evolution:%s", note))
	}

	return evolved, nil
}

// applyAddRule adds a new rule to the recipe
func (re *DefaultRecipeEvolution) applyAddRule(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add a new step for the rule modification
	newStep := models.RecipeStep{
		Name: fmt.Sprintf("Added Rule: %s - %s", mod.Target, mod.Change),
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"rule":        mod.Change,
			"description": mod.Change,
		},
	}
	recipe.Steps = append(recipe.Steps, newStep)
	return recipe
}

// applyModifyRule modifies an existing rule in the recipe
func (re *DefaultRecipeEvolution) applyModifyRule(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// In a real implementation, this would modify existing steps
	// For now, add metadata to track modifications
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("modified:%s", mod.Target))
	return recipe
}

// applyAddCondition adds a condition to the recipe
func (re *DefaultRecipeEvolution) applyAddCondition(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add condition to recipe metadata
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("condition:%s", mod.Change))
	return recipe
}

// applyAddException adds an exception to the recipe
func (re *DefaultRecipeEvolution) applyAddException(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add exception to recipe metadata
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("exception:%s", mod.Change))
	return recipe
}

// applyAdjustPattern adjusts pattern matching in the recipe
func (re *DefaultRecipeEvolution) applyAdjustPattern(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add pattern adjustment to recipe metadata
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("pattern:%s", mod.Change))
	return recipe
}

// applyExtendScope extends the scope of the recipe
func (re *DefaultRecipeEvolution) applyExtendScope(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add scope extension to recipe metadata
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("extended-scope:%s", mod.Change))
	return recipe
}

// applyReduceScope reduces the scope of the recipe
func (re *DefaultRecipeEvolution) applyReduceScope(recipe *models.Recipe, mod RecipeModification) *models.Recipe {
	// Add scope reduction to recipe metadata
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("reduced-scope:%s", mod.Change))
	return recipe
}

// appendToOptionArray is no longer needed with the new models.Recipe structure
func (re *DefaultRecipeEvolution) appendToOptionArray(recipe *models.Recipe, key, value string) *models.Recipe {
	// This function is deprecated - modifications are now tracked via tags
	recipe.Metadata.Tags = append(recipe.Metadata.Tags, fmt.Sprintf("%s:%s", key, value))
	return recipe
}
