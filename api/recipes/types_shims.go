package recipes

import "time"

// SandboxManager is a minimal interface to accept sandbox managers from callers.
// It is intentionally empty to avoid tight coupling; any type satisfies it.
type SandboxManager interface{}

// RiskLevel represents the risk level of a modification.
type RiskLevel int

const (
	RiskLevelLow RiskLevel = iota
	RiskLevelMedium
	RiskLevelModerate
	RiskLevelHigh
	RiskLevelCritical
)

// Codebase is a placeholder for codebase context used in analysis.
type Codebase struct{}

// ErrorContext is a placeholder for error context used in evolution/analysis.
type ErrorContext struct {
	ErrorMessage string `json:"error_message,omitempty"`
	File         string `json:"file,omitempty"`
	Line         int    `json:"line,omitempty"`
}

// LLMAnalysisResult is a minimal placeholder for LLM analysis results.
type LLMAnalysisResult struct {
	ErrorType        string   `json:"error_type"`
	Confidence       float64  `json:"confidence"`
	SuggestedFix     string   `json:"suggested_fix"`
	AlternativeFixes []string `json:"alternative_fixes"`
	RiskAssessment   string   `json:"risk_assessment"`
}

// TransformationResult is a minimal transformation result used by the recipes executor.
type TransformationResult struct {
	RecipeID        string                `json:"recipe_id"`
	Success         bool                  `json:"success"`
	ChangesApplied  int                   `json:"changes_applied"`
	TotalFiles      int                   `json:"total_files,omitempty"`
	FilesModified   []string              `json:"files_modified"`
	Diff            string                `json:"diff"`
	ValidationScore float64               `json:"validation_score,omitempty"`
	ExecutionTime   time.Duration         `json:"execution_time"`
	Errors          []TransformationError `json:"errors,omitempty"`
}

// TransformationError represents an error encountered during transformation.
type TransformationError struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Recoverable bool   `json:"recoverable"`
}
