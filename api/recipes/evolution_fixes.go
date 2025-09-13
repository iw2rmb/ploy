package recipes

import (
	"fmt"
	"strings"
)

// generateSuggestedFixes creates modification suggestions based on analysis
func (re *DefaultRecipeEvolution) generateSuggestedFixes(failure TransformationFailure, patterns []FailurePattern) ([]RecipeModification, float64) {
	var modifications []RecipeModification
	totalConfidence := 0.0

	errorType := re.classifyError(failure)

	switch errorType {
	case ErrorCompilationFailure:
		modifications = append(modifications, re.generateCompilationFixes(failure)...)
	case ErrorDependencyIssue:
		modifications = append(modifications, re.generateDependencyFixes(failure)...)
	case ErrorIncompleteTransform:
		modifications = append(modifications, re.generateCompletenesssFixes(failure)...)
	case ErrorRecipeMismatch:
		modifications = append(modifications, re.generatePatternFixes(failure)...)
	case ErrorSemanticChange:
		modifications = append(modifications, re.generateSemanticFixes(failure)...)
	case ErrorTimeoutFailure:
		modifications = append(modifications, re.generateTimeoutFixes(failure)...)
	case ErrorResourceExhaustion:
		modifications = append(modifications, re.generateResourceFixes(failure)...)
	}

	// Add fixes from similar patterns
	for _, pattern := range patterns {
		if pattern.Mitigations[0] != "" {
			mod := RecipeModification{
				Type:          ModificationAddRule,
				Target:        "pattern_based_fix",
				Change:        pattern.Mitigations[0],
				Justification: fmt.Sprintf("Based on successful fix for similar pattern (frequency: %d)", pattern.Frequency),
				Priority:      5,
				RiskLevel:     RiskLevelModerate,
			}
			modifications = append(modifications, mod)
		}
	}

	// Calculate overall confidence based on modification quality
	if len(modifications) > 0 {
		for _, mod := range modifications {
			confidence := re.calculateModificationConfidence(mod)
			totalConfidence += confidence
		}
		totalConfidence /= float64(len(modifications))
	}

	return modifications, totalConfidence
}

// generateCompilationFixes creates fixes for compilation failures
func (re *DefaultRecipeEvolution) generateCompilationFixes(failure TransformationFailure) []RecipeModification {
	var modifications []RecipeModification

	if strings.Contains(failure.ErrorMessage, "cannot find symbol") {
		modifications = append(modifications, RecipeModification{
			Type:          ModificationAddRule,
			Target:        "import_resolution",
			Change:        "Add missing import detection and automatic import addition",
			Justification: "Compilation failure due to missing symbol suggests missing imports",
			Priority:      1,
			RiskLevel:     RiskLevelLow,
		})
	}

	if strings.Contains(failure.ErrorMessage, "incompatible types") {
		modifications = append(modifications, RecipeModification{
			Type:          ModificationAddCondition,
			Target:        "type_compatibility",
			Change:        "Add type compatibility check before transformation",
			Justification: "Type incompatibility suggests recipe needs type validation",
			Priority:      2,
			RiskLevel:     RiskLevelModerate,
		})
	}

	return modifications
}

// generateDependencyFixes creates fixes for dependency issues
func (re *DefaultRecipeEvolution) generateDependencyFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationAddRule,
			Target:        "dependency_validation",
			Change:        "Add dependency availability check",
			Justification: "Dependency errors suggest need for pre-transformation dependency validation",
			Priority:      1,
			RiskLevel:     RiskLevelLow,
		},
		{
			Type:          ModificationAddException,
			Target:        "missing_dependencies",
			Change:        "Skip transformation when required dependencies are missing",
			Justification: "Graceful handling of missing dependencies prevents failures",
			Priority:      3,
			RiskLevel:     RiskLevelLow,
		},
	}
}

// generateCompletenesssFixes creates fixes for incomplete transformations
func (re *DefaultRecipeEvolution) generateCompletenesssFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationExtendScope,
			Target:        "transformation_scope",
			Change:        "Extend pattern matching to cover additional code patterns",
			Justification: "Incomplete transformation suggests limited pattern coverage",
			Priority:      2,
			RiskLevel:     RiskLevelModerate,
		},
		{
			Type:          ModificationAddRule,
			Target:        "completeness_check",
			Change:        "Add post-transformation completeness validation",
			Justification: "Ensure all intended transformations are applied",
			Priority:      3,
			RiskLevel:     RiskLevelLow,
		},
	}
}

// generatePatternFixes creates fixes for pattern matching issues
func (re *DefaultRecipeEvolution) generatePatternFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationAdjustPattern,
			Target:        "matching_patterns",
			Change:        "Broaden pattern matching criteria",
			Justification: "Pattern mismatch suggests patterns are too restrictive",
			Priority:      2,
			RiskLevel:     RiskLevelModerate,
		},
		{
			Type:          ModificationAddCondition,
			Target:        "pattern_validation",
			Change:        "Add pre-check for pattern applicability",
			Justification: "Validate patterns before attempting transformation",
			Priority:      1,
			RiskLevel:     RiskLevelLow,
		},
	}
}

// generateSemanticFixes creates fixes for semantic change issues
func (re *DefaultRecipeEvolution) generateSemanticFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationAddRule,
			Target:        "semantic_preservation",
			Change:        "Add semantic equivalence validation",
			Justification: "Semantic change errors require validation of behavior preservation",
			Priority:      1,
			RiskLevel:     RiskLevelHigh,
		},
		{
			Type:          ModificationReduceScope,
			Target:        "transformation_scope",
			Change:        "Limit transformation to safer, more conservative changes",
			Justification: "Reduce risk of semantic changes",
			Priority:      2,
			RiskLevel:     RiskLevelModerate,
		},
	}
}

// generateTimeoutFixes creates fixes for timeout issues
func (re *DefaultRecipeEvolution) generateTimeoutFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationReduceScope,
			Target:        "processing_scope",
			Change:        "Reduce processing scope to improve performance",
			Justification: "Timeout suggests processing is too intensive",
			Priority:      1,
			RiskLevel:     RiskLevelLow,
		},
		{
			Type:          ModificationAddCondition,
			Target:        "size_limit",
			Change:        "Add file size or complexity limits",
			Justification: "Prevent timeout on overly large or complex files",
			Priority:      2,
			RiskLevel:     RiskLevelLow,
		},
	}
}

// generateResourceFixes creates fixes for resource exhaustion
func (re *DefaultRecipeEvolution) generateResourceFixes(failure TransformationFailure) []RecipeModification {
	return []RecipeModification{
		{
			Type:          ModificationAddCondition,
			Target:        "resource_limits",
			Change:        "Add memory and CPU usage limits",
			Justification: "Resource exhaustion requires usage limits",
			Priority:      1,
			RiskLevel:     RiskLevelLow,
		},
		{
			Type:          ModificationModifyRule,
			Target:        "processing_strategy",
			Change:        "Use streaming or batched processing for large inputs",
			Justification: "Reduce memory footprint for large transformations",
			Priority:      2,
			RiskLevel:     RiskLevelModerate,
		},
	}
}

// calculateModificationConfidence estimates confidence in a specific modification
func (re *DefaultRecipeEvolution) calculateModificationConfidence(mod RecipeModification) float64 {
	confidence := 0.5 // Base confidence

	// Adjust based on risk level
	switch mod.RiskLevel {
	case RiskLevelLow:
		confidence += 0.3
	case RiskLevelModerate:
		confidence += 0.1
	case RiskLevelHigh:
		confidence -= 0.2
	}

	// Adjust based on modification type
	switch mod.Type {
	case ModificationAddCondition, ModificationAddException:
		confidence += 0.2 // Generally safe additions
	case ModificationModifyRule, ModificationAdjustPattern:
		confidence += 0.1 // Moderate confidence
	case ModificationRemoveRule:
		confidence -= 0.1 // Riskier
	}

	// Ensure confidence is within valid range
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}
