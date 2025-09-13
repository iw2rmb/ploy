package mods

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/api/arf"
)

// SelfHealConfig represents the self-healing configuration in mods.yaml
type SelfHealConfig struct {
	MaxRetries int    `yaml:"max_retries"`
	Cooldown   string `yaml:"cooldown"`
	Enabled    bool   `yaml:"enabled"`
}

// Validate checks if the self-healing configuration is valid
func (c *SelfHealConfig) Validate() error {
	if c.MaxRetries < 0 {
		return errors.New("max_retries cannot be negative")
	}
	if c.MaxRetries > 5 {
		return errors.New("max_retries cannot exceed 5")
	}

	if c.Cooldown != "" {
		if _, err := time.ParseDuration(c.Cooldown); err != nil {
			return fmt.Errorf("invalid cooldown format: %v", err)
		}
	}

	return nil
}

// ParseCooldown parses the cooldown duration
func (c *SelfHealConfig) ParseCooldown() (time.Duration, error) {
	if c.Cooldown == "" {
		return 0, nil
	}
	return time.ParseDuration(c.Cooldown)
}

// GetDefaults returns a default self-healing configuration
func GetDefaultSelfHealConfig() *SelfHealConfig {
	return &SelfHealConfig{
		MaxRetries: 1,
		Cooldown:   "",
		Enabled:    true,
	}
}

// ModHealingAttempt represents a single healing attempt in the Mods context
// This is simpler than the complex ARF HealingAttempt and focused on Mods needs
type ModHealingAttempt struct {
	AttemptNumber    int              `json:"attempt_number"`
	ErrorContext     arf.ErrorContext `json:"error_context"`
	SuggestedRecipes []string         `json:"suggested_recipes"`
	AppliedRecipes   []string         `json:"applied_recipes"`
	Success          bool             `json:"success"`
	ErrorMessage     string           `json:"error_message,omitempty"`
	Duration         time.Duration    `json:"duration"`
	Timestamp        time.Time        `json:"timestamp"`
}

// NewModHealingAttempt creates a new healing attempt
func NewModHealingAttempt(attemptNumber int, errorContext arf.ErrorContext, suggestedRecipes []string) *ModHealingAttempt {
	return &ModHealingAttempt{
		AttemptNumber:    attemptNumber,
		ErrorContext:     errorContext,
		SuggestedRecipes: suggestedRecipes,
		AppliedRecipes:   []string{},
		Success:          false,
		Timestamp:        time.Now(),
	}
}

// MarkSuccess marks the healing attempt as successful
func (a *ModHealingAttempt) MarkSuccess(appliedRecipes []string, duration time.Duration) {
	a.Success = true
	a.AppliedRecipes = appliedRecipes
	a.Duration = duration
	a.ErrorMessage = ""
}

// MarkFailure marks the healing attempt as failed
func (a *ModHealingAttempt) MarkFailure(appliedRecipes []string, errorMsg string, duration time.Duration) {
	a.Success = false
	a.AppliedRecipes = appliedRecipes
	a.ErrorMessage = errorMsg
	a.Duration = duration
}

// ModHealingSummary tracks the overall healing process for a Mod run
type ModHealingSummary struct {
	Enabled       bool                 `json:"enabled"`
	AttemptsCount int                  `json:"attempts_count"`
	MaxRetries    int                  `json:"max_retries"`
	Attempts      []*ModHealingAttempt `json:"attempts"`
	FinalSuccess  bool                 `json:"final_success"`
	TotalHealed   int                  `json:"total_healed"`
	TotalDuration time.Duration        `json:"total_duration"`

	// Job-based healing workflow fields
	PlanID     string         `json:"plan_id,omitempty"`
	Winner     *BranchResult  `json:"winner,omitempty"`
	AllResults []BranchResult `json:"all_results,omitempty"`
}

// NewModHealingSummary creates a new healing summary
func NewModHealingSummary(enabled bool, maxRetries int) *ModHealingSummary {
	return &ModHealingSummary{
		Enabled:       enabled,
		AttemptsCount: 0,
		MaxRetries:    maxRetries,
		Attempts:      []*ModHealingAttempt{},
		FinalSuccess:  false,
		TotalHealed:   0,
	}
}

// AddAttempt adds a healing attempt to the summary
func (s *ModHealingSummary) AddAttempt(attempt *ModHealingAttempt) {
	s.Attempts = append(s.Attempts, attempt)
	s.AttemptsCount++
	s.TotalDuration += attempt.Duration

	if attempt.Success {
		s.TotalHealed++
	}
}

// SetFinalResult sets the final success status
func (s *ModHealingSummary) SetFinalResult(success bool) {
	s.FinalSuccess = success
}

// HasReachedMaxRetries checks if max retries have been reached
func (s *ModHealingSummary) HasReachedMaxRetries() bool {
	return s.AttemptsCount >= s.MaxRetries
}

// BuildFailureAnalysis represents the analysis of build failure for recipe suggestions
type BuildFailureAnalysis struct {
	SuggestedRecipes []string `json:"suggested_recipes"`
	Confidence       float64  `json:"confidence"`
	ErrorType        string   `json:"error_type"`
	Language         string   `json:"language,omitempty"`
	SourceFiles      []string `json:"source_files,omitempty"`
}

// ModErrorAnalyzer analyzes build failures and suggests healing recipes
type ModErrorAnalyzer struct {
	// In the future, this might wrap the ARF analyzer
	// For now, we'll use simple pattern matching
}

// NewModErrorAnalyzer creates a new error analyzer for Mods
func NewModErrorAnalyzer() *ModErrorAnalyzer {
	return &ModErrorAnalyzer{}
}

// AnalyzeBuildFailure analyzes build failure and suggests healing recipes
func (a *ModErrorAnalyzer) AnalyzeBuildFailure(ctx context.Context, errors []string, language string) (*BuildFailureAnalysis, error) {
	// For MVP, use simple error pattern matching to suggest common recipes
	// In the future, integrate with the full ARF LLM analyzer

	errorContext := arf.ExtractErrorContext(errors, language)

	analysis := &BuildFailureAnalysis{
		ErrorType:        errorContext.ErrorType,
		Language:         language,
		Confidence:       0.7, // Default confidence for pattern matching
		SuggestedRecipes: []string{},
	}

	// Simple pattern-based recipe suggestions
	switch errorContext.ErrorType {
	case "import":
		analysis.SuggestedRecipes = append(analysis.SuggestedRecipes, "org.openrewrite.java.RemoveUnusedImports")
		analysis.Confidence = 0.8

	case "compilation":
		// Suggest common compilation fixes
		switch language {
		case "java":
			analysis.SuggestedRecipes = append(analysis.SuggestedRecipes,
				"org.openrewrite.java.cleanup.SimplifyBooleanExpression",
				"org.openrewrite.java.cleanup.UnnecessaryParentheses")
		case "go":
			analysis.SuggestedRecipes = append(analysis.SuggestedRecipes,
				"org.openrewrite.go.format.AutoFormat",
				"org.openrewrite.go.cleanup.UnnecessaryParentheses")
		}
		analysis.Confidence = 0.6

	case "dependency":
		analysis.SuggestedRecipes = append(analysis.SuggestedRecipes, "org.openrewrite.java.dependencies.DependencyVulnerabilityCheck")
		analysis.Confidence = 0.5

	default:
		// For unknown errors, suggest general cleanup recipes
		if language == "java" {
			analysis.SuggestedRecipes = append(analysis.SuggestedRecipes,
				"org.openrewrite.java.cleanup.CommonStaticAnalysis")
		}
		analysis.Confidence = 0.4
	}

	// Extract source files if available
	if errorContext.SourceFile != "" {
		analysis.SourceFiles = []string{errorContext.SourceFile}
	}

	return analysis, nil
}
