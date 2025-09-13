package arf

import (
	"context"
	"fmt"

	recipes "github.com/iw2rmb/ploy/api/recipes"
)

// HealingSuggestionService provides healing suggestion creation and management
type HealingSuggestionService struct {
	recipeConverter *recipes.RecipeConverter
}

// NewHealingSuggestionService creates a new healing suggestion service
func NewHealingSuggestionService() *HealingSuggestionService {
	return &HealingSuggestionService{
		recipeConverter: recipes.NewRecipeConverter(),
	}
}

// HealingSuggestion represents a complete healing suggestion with recipe
type HealingSuggestion struct {
	Analysis        *LLMAnalysisResult     `json:"analysis"`
	RecipeName      string                 `json:"recipe_name"`
	RecipeMetadata  map[string]interface{} `json:"recipe_metadata"`
	SandboxID       string                 `json:"sandbox_id"`
	Language        string                 `json:"language"`
	Confidence      float64                `json:"confidence"`
	EstimatedImpact string                 `json:"estimated_impact"`
	Prerequisites   []string               `json:"prerequisites"`
}

// CreateHealingSuggestion creates a complete healing suggestion from analysis results
func (hs *HealingSuggestionService) CreateHealingSuggestion(ctx context.Context, analysis *LLMAnalysisResult, language string, sandboxID string) (*HealingSuggestion, error) {
	if analysis == nil {
		return nil, fmt.Errorf("analysis cannot be nil")
	}

	// Convert to OpenRewrite recipe (adapt analysis to recipes type)
	rcAnalysis := &recipes.LLMAnalysisResult{
		ErrorType:        analysis.ErrorType,
		Confidence:       analysis.Confidence,
		SuggestedFix:     analysis.SuggestedFix,
		AlternativeFixes: analysis.AlternativeFixes,
		RiskAssessment:   analysis.RiskAssessment,
	}
	recipeName, recipeMetadata := hs.recipeConverter.ConvertToOpenRewriteRecipe(rcAnalysis, language)

	// Create healing suggestion
	suggestion := &HealingSuggestion{
		Analysis:        analysis,
		RecipeName:      recipeName,
		RecipeMetadata:  recipeMetadata,
		SandboxID:       sandboxID,
		Language:        language,
		Confidence:      analysis.Confidence,
		EstimatedImpact: hs.estimateImpact(analysis),
		Prerequisites:   hs.determinePrerequisites(analysis, language),
	}

	return suggestion, nil
}

// estimateImpact estimates the impact of applying the healing suggestion
func (hs *HealingSuggestionService) estimateImpact(analysis *LLMAnalysisResult) string {
	switch analysis.RiskAssessment {
	case "high":
		return "major"
	case "medium":
		return "moderate"
	default:
		return "minor"
	}
}

// determinePrerequisites determines what needs to be in place before applying the fix
func (hs *HealingSuggestionService) determinePrerequisites(analysis *LLMAnalysisResult, language string) []string {
	prereqs := []string{}

	if analysis.ErrorType == "import" || analysis.ErrorType == "dependency" {
		switch language {
		case "java":
			prereqs = append(prereqs, "Maven or Gradle build file updated")
		case "python":
			prereqs = append(prereqs, "requirements.txt or setup.py updated")
		case "go":
			prereqs = append(prereqs, "go.mod updated")
		case "javascript", "typescript":
			prereqs = append(prereqs, "package.json updated")
		}
	}

	if analysis.ErrorType == "test" {
		prereqs = append(prereqs, "Test data validated")
		prereqs = append(prereqs, "Business requirements confirmed")
	}

	return prereqs
}
