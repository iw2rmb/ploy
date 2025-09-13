package recipes

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// AnalyzeFailure performs comprehensive analysis of a transformation failure
func (re *DefaultRecipeEvolution) AnalyzeFailure(ctx context.Context, failure TransformationFailure) (*FailureAnalysis, error) {
	analysis := &FailureAnalysis{
		ErrorType:     re.classifyError(failure),
		RootCause:     re.identifyRootCause(failure),
		ContextInfo:   failure.Context,
		AffectedFiles: failure.FailedFiles,
		AnalysisTime:  time.Now(),
	}

	// Pattern learning disabled - no similar patterns

	// Generate suggested fixes based on error type and patterns
	fixes, confidence := re.generateSuggestedFixes(failure, analysis.SimilarPatterns)
	analysis.SuggestedFixes = fixes
	analysis.Confidence = confidence

	return analysis, nil
}

// classifyError determines the type of error from the failure details
func (re *DefaultRecipeEvolution) classifyError(failure TransformationFailure) ErrorType {
	errorMsg := strings.ToLower(failure.ErrorMessage)

	// Check for specific error patterns
	if strings.Contains(errorMsg, "compilation") || strings.Contains(errorMsg, "compile") {
		return ErrorCompilationFailure
	}

	if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline") {
		return ErrorTimeoutFailure
	}

	if strings.Contains(errorMsg, "memory") || strings.Contains(errorMsg, "oom") || strings.Contains(errorMsg, "resource") {
		return ErrorResourceExhaustion
	}

	if strings.Contains(errorMsg, "dependency") || strings.Contains(errorMsg, "import") || strings.Contains(errorMsg, "package") {
		return ErrorDependencyIssue
	}

	if strings.Contains(errorMsg, "semantic") || strings.Contains(errorMsg, "behavior") {
		return ErrorSemanticChange
	}

	if strings.Contains(errorMsg, "incomplete") || strings.Contains(errorMsg, "partial") {
		return ErrorIncompleteTransform
	}

	if strings.Contains(errorMsg, "pattern") || strings.Contains(errorMsg, "match") || strings.Contains(errorMsg, "recipe") {
		return ErrorRecipeMismatch
	}

	return ErrorUnknown
}

// identifyRootCause extracts the root cause from error details
func (re *DefaultRecipeEvolution) identifyRootCause(failure TransformationFailure) string {
	// Use regex to extract meaningful error information
	errorMsg := failure.ErrorMessage

	// Common Java compilation error patterns
	javaErrorPatterns := []string{
		`cannot find symbol.*`,
		`incompatible types.*`,
		`unreachable statement`,
		`variable .* might not have been initialized`,
		`method .* in class .* cannot be applied`,
	}

	for _, pattern := range javaErrorPatterns {
		if match, _ := regexp.MatchString(pattern, errorMsg); match {
			// Extract the specific error details
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(errorMsg); len(matches) > 0 {
				return strings.TrimSpace(matches[0])
			}
		}
	}

	// Fall back to first line of error message
	lines := strings.Split(errorMsg, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return "Unknown error"
}

// convertStringMap converts map[string]string to map[string]interface{}
func convertStringMap(stringMap map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range stringMap {
		result[k] = v
	}
	return result
}
