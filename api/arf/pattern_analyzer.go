package arf

import (
	"strings"
)

// PatternAnalyzer provides pattern-based error analysis as fallback when LLM is unavailable
type PatternAnalyzer struct{}

// NewPatternAnalyzer creates a new pattern analyzer
func NewPatternAnalyzer() *PatternAnalyzer {
	return &PatternAnalyzer{}
}

// AnalyzeErrors performs pattern-based error analysis as fallback
func (p *PatternAnalyzer) AnalyzeErrors(errors []string, language string) *LLMAnalysisResult {
	errorText := strings.ToLower(strings.Join(errors, " "))

	result := &LLMAnalysisResult{
		ErrorType:        "unknown",
		Confidence:       0.5,
		AlternativeFixes: []string{},
		RiskAssessment:   "medium",
	}

	// Java patterns
	if language == "java" {
		if strings.Contains(errorText, "cannot find symbol") {
			result.ErrorType = "compilation"
			result.Confidence = 0.8
			result.SuggestedFix = "Add missing import statement or define the missing class/method"
			result.AlternativeFixes = []string{
				"Check if the required dependency is in your pom.xml or build.gradle",
				"Verify the class name spelling and package structure",
			}
			result.RiskAssessment = "low"
		} else if strings.Contains(errorText, "package") && strings.Contains(errorText, "does not exist") {
			result.ErrorType = "import"
			result.Confidence = 0.85
			result.SuggestedFix = "Add the missing package dependency to your build file"
			result.AlternativeFixes = []string{
				"Create the missing package structure",
				"Update import statements to use correct package names",
			}
			result.RiskAssessment = "low"
		}
	}

	// Python patterns
	if language == "python" {
		if strings.Contains(errorText, "modulenotfounderror") || strings.Contains(errorText, "no module named") {
			result.ErrorType = "import"
			result.Confidence = 0.9
			result.SuggestedFix = "Install the missing module using pip install"
			result.AlternativeFixes = []string{
				"Add the module to requirements.txt",
				"Check if the module name is spelled correctly",
			}
			result.RiskAssessment = "low"
		} else if strings.Contains(errorText, "syntaxerror") {
			result.ErrorType = "syntax"
			result.Confidence = 0.75
			result.SuggestedFix = "Fix the syntax error at the indicated line"
			result.AlternativeFixes = []string{
				"Check for missing colons, parentheses, or indentation",
			}
			result.RiskAssessment = "low"
		}
	}

	// Go patterns
	if language == "go" {
		if strings.Contains(errorText, "undefined") {
			result.ErrorType = "compilation"
			result.Confidence = 0.8
			result.SuggestedFix = "Import the required package or define the missing identifier"
			result.AlternativeFixes = []string{
				"Run 'go get' to fetch missing dependencies",
				"Check if the identifier is exported (capitalized)",
			}
			result.RiskAssessment = "low"
		} else if strings.Contains(errorText, "cannot use") && strings.Contains(errorText, "as type") {
			result.ErrorType = "type_mismatch"
			result.Confidence = 0.85
			result.SuggestedFix = "Fix type mismatch by converting or changing the variable type"
			result.AlternativeFixes = []string{
				"Use type assertion or type conversion",
				"Update function signature to match expected types",
			}
			result.RiskAssessment = "medium"
		}
	}

	// Test failure patterns (language agnostic)
	if strings.Contains(errorText, "test") &&
		(strings.Contains(errorText, "fail") || strings.Contains(errorText, "assertion")) {
		result.ErrorType = "test"
		result.Confidence = 0.7
		result.SuggestedFix = "Review the test logic and expected values"
		result.AlternativeFixes = []string{
			"Update test expectations if business logic changed",
			"Fix the implementation to match test expectations",
			"Check for race conditions or timing issues",
		}
		result.RiskAssessment = "medium"
	}

	return result
}
