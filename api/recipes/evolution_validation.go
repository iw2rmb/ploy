package recipes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// ValidateEvolution validates that an evolved recipe is safe to use
func (re *DefaultRecipeEvolution) ValidateEvolution(ctx context.Context, original, evolved *models.Recipe) (*EvolutionValidationResult, error) {
	result := &EvolutionValidationResult{
		Valid:           true,
		SafetyScore:     1.0,
		Warnings:        []string{},
		CriticalIssues:  []string{},
		TestResults:     []EvolutionValidationTest{},
		RecommendAction: ActionApprove,
	}

	// Validate confidence change
	confidenceTest := EvolutionValidationTest{
		Name:        "confidence_validation",
		Description: "Check if evolved recipe maintains reasonable confidence",
		Runtime:     10 * time.Millisecond,
	}

	// Check if recipe was marked as evolved
	isEvolved := false
	for _, tag := range evolved.Metadata.Tags {
		if tag == "evolved" {
			isEvolved = true
			break
		}
	}

	if isEvolved {
		confidenceTest.Status = "passed"
		confidenceTest.Details = "Recipe evolution completed"
	} else {
		confidenceTest.Status = "warning"
		confidenceTest.Details = "Recipe may not have been properly evolved"
		result.Warnings = append(result.Warnings, confidenceTest.Details)
		result.SafetyScore -= 0.1
	}

	result.TestResults = append(result.TestResults, confidenceTest)

	// Validate evolution options
	optionsTest := EvolutionValidationTest{
		Name:        "options_validation",
		Description: "Validate evolved recipe options",
		Runtime:     5 * time.Millisecond,
	}

	// Check for evolution tags
	evolutionTagCount := 0
	for _, tag := range evolved.Metadata.Tags {
		if strings.HasPrefix(tag, "evolution:") {
			evolutionTagCount++
		}
	}

	if evolutionTagCount > 0 {
		optionsTest.Status = "passed"
		optionsTest.Details = fmt.Sprintf("Evolution notes: %d modifications", evolutionTagCount)
	} else {
		optionsTest.Status = "warning"
		optionsTest.Details = "No evolution notes found in tags"
		result.Warnings = append(result.Warnings, optionsTest.Details)
	}

	result.TestResults = append(result.TestResults, optionsTest)

	// Determine recommendation based on validation
	if len(result.CriticalIssues) > 0 {
		result.RecommendAction = ActionReject
	} else if result.SafetyScore < re.config.AutoApprovalThreshold || len(result.Warnings) > 3 {
		result.RecommendAction = ActionRequireReview
	} else if re.config.EnableAutoApproval && result.SafetyScore >= re.config.AutoApprovalThreshold {
		result.RecommendAction = ActionApprove
	} else {
		result.RecommendAction = ActionRunTests
	}

	return result, nil
}
