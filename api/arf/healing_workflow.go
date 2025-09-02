package arf

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HealingConfig contains configuration for healing workflows
type HealingConfig struct {
	MaxHealingDepth     int           `json:"max_healing_depth"`     // Maximum nesting depth (default: 5)
	MaxParallelAttempts int           `json:"max_parallel_attempts"` // Max concurrent healing (default: 3)
	MaxTotalAttempts    int           `json:"max_total_attempts"`    // Max total attempts per root (default: 20)
	HealingTimeout      time.Duration `json:"healing_timeout"`       // Total healing timeout (default: 2h)
	AttemptTimeout      time.Duration `json:"attempt_timeout"`       // Per-attempt timeout (default: 30m)
	EnableHealing       bool          `json:"enable_healing"`        // Enable/disable healing

	// Circuit breaker settings
	FailureThreshold    int           `json:"failure_threshold"`     // Consecutive failures before circuit open
	CircuitOpenDuration time.Duration `json:"circuit_open_duration"` // How long circuit stays open

	// Validation settings
	ValidateBuild        bool          `json:"validate_build"`         // Enable build validation (default: true)
	ValidateTests        bool          `json:"validate_tests"`         // Enable test validation (default: true)
	BuildTimeout         time.Duration `json:"build_timeout"`          // Build validation timeout (default: 10m)
	TestTimeout          time.Duration `json:"test_timeout"`           // Test validation timeout (default: 15m)
	DefaultBuildTool     string        `json:"default_build_tool"`     // Default build tool (maven, gradle, npm, go)
	DefaultTestFramework string        `json:"default_test_framework"` // Default test framework
}

// DefaultHealingConfig returns default healing configuration
func DefaultHealingConfig() *HealingConfig {
	return &HealingConfig{
		MaxHealingDepth:      5,
		MaxParallelAttempts:  3,
		MaxTotalAttempts:     20,
		HealingTimeout:       2 * time.Hour,
		AttemptTimeout:       30 * time.Minute,
		EnableHealing:        true,
		FailureThreshold:     3,
		CircuitOpenDuration:  5 * time.Minute,
		ValidateBuild:        true,
		ValidateTests:        true,
		BuildTimeout:         10 * time.Minute,
		TestTimeout:          15 * time.Minute,
		DefaultBuildTool:     "maven",
		DefaultTestFramework: "maven",
	}
}

// executeHealingWorkflow executes a recursive healing workflow for transformation errors
func (h *Handler) executeHealingWorkflow(
	ctx context.Context,
	transformID string,
	errors []string,
	parentPath string,
	config *HealingConfig,
) error {
	// Check if healing is enabled
	if config == nil {
		config = DefaultHealingConfig()
	}

	if !config.EnableHealing {
		return fmt.Errorf("healing workflow is disabled")
	}

	// Check depth limit
	if !h.canPerformHealing(parentPath, config) {
		return fmt.Errorf("max healing depth (%d) reached", config.MaxHealingDepth)
	}

	// Generate attempt path
	attemptPath, err := h.consulStore.GenerateNextAttemptPath(ctx, transformID, parentPath)
	if err != nil {
		return fmt.Errorf("failed to generate attempt path: %w", err)
	}

	// Analyze errors with LLM
	analysis := h.analyzeBuildErrors(errors)

	// Create healing attempt
	attempt := &HealingAttempt{
		TransformationID: uuid.New().String(),
		AttemptPath:      attemptPath,
		TriggerReason:    h.determineTriggerReason(errors),
		TargetErrors:     errors,
		LLMAnalysis:      analysis,
		Status:           "in_progress",
		StartTime:        time.Now(),
		Children:         []HealingAttempt{},
		ParentAttempt:    parentPath,
	}

	// Store in Consul
	if err := h.consulStore.AddHealingAttempt(ctx, transformID, attemptPath, attempt); err != nil {
		return fmt.Errorf("failed to store healing attempt: %w", err)
	}

	// Apply the healing transformation
	healingCtx, cancel := context.WithTimeout(ctx, config.AttemptTimeout)
	defer cancel()

	healingSuccess := false
	var healingError error

	// Generate healing recipe using LLM
	if h.llmGenerator != nil && analysis != nil {
		recipeRequest := RecipeGenerationRequest{
			ErrorContext: ErrorContext{
				ErrorType:    analysis.ErrorType,
				ErrorMessage: strings.Join(errors, "\n"),
			},
			Language: "java", // TODO: Detect language from transformation context
		}

		generatedRecipe, err := h.llmGenerator.GenerateRecipe(healingCtx, recipeRequest)
		if err == nil && generatedRecipe != nil {
			// Apply the generated recipe
			// TODO: Implement recipe application
			healingSuccess = true
			attempt.Status = "completed"
			attempt.Result = "success"
		} else {
			healingError = err
			attempt.Status = "completed"
			attempt.Result = "failed"
		}
	} else {
		attempt.Status = "completed"
		attempt.Result = "failed"
		healingError = fmt.Errorf("LLM generator not available")
	}

	attempt.EndTime = time.Now()

	// Check for new issues after healing
	if healingSuccess {
		newErrors := h.validateAfterHealing(attempt.TransformationID)
		if len(newErrors) > 0 {
			attempt.NewIssuesDiscovered = newErrors
			attempt.Result = "partial_success"

			// Spawn child healing attempts for new issues (recursive)
			go func() {
				childCtx := context.Background() // Use new context for child healing
				if err := h.executeHealingWorkflow(childCtx, transformID, newErrors, attemptPath, config); err != nil {
					fmt.Printf("Child healing workflow failed: %v\n", err)
				}
			}()
		}
	}

	// Update Consul with final status
	if err := h.consulStore.UpdateHealingAttempt(ctx, transformID, attemptPath, attempt); err != nil {
		return fmt.Errorf("failed to update healing attempt: %w", err)
	}

	if healingError != nil {
		return fmt.Errorf("healing attempt failed: %w", healingError)
	}

	return nil
}

// analyzeBuildErrors analyzes build/test errors using LLM to suggest fixes
func (h *Handler) analyzeBuildErrors(errors []string) *LLMAnalysisResult {
	if len(errors) == 0 {
		return nil
	}

	// Categorize error type
	errorType := "unknown"
	prompt := "Fix the following errors:\n"

	// Check for compilation errors
	for _, err := range errors {
		if strings.Contains(strings.ToLower(err), "compilation") ||
			strings.Contains(strings.ToLower(err), "cannot find symbol") ||
			strings.Contains(strings.ToLower(err), "error:") {
			errorType = "compilation"
			prompt = "Fix the following compilation errors:\n"
			break
		}
	}

	// Check for test failures
	if errorType == "unknown" {
		for _, err := range errors {
			if strings.Contains(strings.ToLower(err), "test") ||
				strings.Contains(strings.ToLower(err), "failed") ||
				strings.Contains(strings.ToLower(err), "assertion") {
				errorType = "test"
				prompt = "Fix the following test failures:\n"
				break
			}
		}
	}

	// Check for mixed errors
	hasCompilation := false
	hasTest := false
	for _, err := range errors {
		if strings.Contains(strings.ToLower(err), "error:") {
			hasCompilation = true
		}
		if strings.Contains(strings.ToLower(err), "test") {
			hasTest = true
		}
	}
	if hasCompilation && hasTest {
		errorType = "mixed"
		prompt = "Fix the following errors:\n"
	}

	// Build suggested fix
	suggestedFix := prompt + strings.Join(errors, "\n")

	return &LLMAnalysisResult{
		ErrorType:        errorType,
		Confidence:       0.7, // Default confidence
		SuggestedFix:     suggestedFix,
		AlternativeFixes: []string{},
		RiskAssessment:   "medium",
	}
}

// determineTriggerReason determines the trigger reason from error messages
func (h *Handler) determineTriggerReason(errors []string) string {
	if len(errors) == 0 {
		return "unknown_failure"
	}

	// Join all errors for analysis
	errorText := strings.ToLower(strings.Join(errors, " "))

	// Check for build failure
	if strings.Contains(errorText, "build failed") ||
		strings.Contains(errorText, "compilation error") ||
		strings.Contains(errorText, "cannot find symbol") {
		return "build_failure"
	}

	// Check for test failure
	if strings.Contains(errorText, "test") &&
		(strings.Contains(errorText, "fail") || strings.Contains(errorText, "error")) {
		return "test_failure"
	}

	// Check for validation failure
	if strings.Contains(errorText, "validation") &&
		strings.Contains(errorText, "fail") {
		return "validation_failure"
	}

	// Check for deployment failure
	if strings.Contains(errorText, "deploy") &&
		strings.Contains(errorText, "fail") {
		return "deployment_failure"
	}

	return "unknown_failure"
}

// validateAfterHealing validates the transformation after applying healing
func (h *Handler) validateAfterHealing(sandboxID string) []string {
	var errors []string

	if h.sandboxMgr == nil {
		return []string{"Sandbox manager not available"}
	}

	ctx := context.Background()
	config := DefaultHealingConfig()

	// Create sandbox validator
	validator := NewSandboxValidator(h.sandboxMgr)

	// Validate build if enabled
	if config.ValidateBuild {
		buildConfig := BuildConfig{
			BuildTool: config.DefaultBuildTool,
			Timeout:   config.BuildTimeout,
		}

		buildResult, err := validator.ValidateBuild(ctx, sandboxID, buildConfig)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Build validation error: %v", err))
		} else if !buildResult.Success {
			for _, buildErr := range buildResult.Errors {
				errors = append(errors, fmt.Sprintf("%s:%d:%d: %s", buildErr.File, buildErr.Line, buildErr.Column, buildErr.Message))
			}
		}
	}

	// Run tests if enabled
	if config.ValidateTests {
		testConfig := TestConfig{
			TestFramework: config.DefaultTestFramework,
			Timeout:       config.TestTimeout,
		}

		testResult, err := validator.RunTests(ctx, sandboxID, testConfig)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Test validation error: %v", err))
		} else if !testResult.Success {
			for _, failure := range testResult.Failures {
				errors = append(errors, fmt.Sprintf("Test %s failed: %s", failure.TestName, failure.Message))
			}
		}
	}

	return errors
}

// canPerformHealing checks if healing can be performed based on depth and other limits
func (h *Handler) canPerformHealing(currentPath string, config *HealingConfig) bool {
	if config == nil {
		return false
	}

	// Check depth limit
	currentDepth := GetPathDepth(currentPath)
	if currentDepth >= config.MaxHealingDepth {
		return false
	}

	// TODO: Check other limits like total attempts, parallel attempts, etc.

	return true
}

// triggerHealingIfNeeded checks if healing should be triggered based on transformation result
func (h *Handler) triggerHealingIfNeeded(
	ctx context.Context,
	transformID string,
	result *TransformationResult,
	config *HealingConfig,
) {
	if config == nil || !config.EnableHealing {
		return
	}

	// Check if transformation failed
	if result == nil || result.Success {
		return
	}

	// Collect errors
	var errors []string
	for _, err := range result.Errors {
		errors = append(errors, err.Message)
	}

	// Also check error log
	for _, err := range result.ErrorLog {
		errors = append(errors, err.Message)
	}

	if len(errors) == 0 {
		return
	}

	// Trigger healing workflow asynchronously
	go func() {
		healingCtx := context.Background()
		if err := h.executeHealingWorkflow(healingCtx, transformID, errors, "", config); err != nil {
			fmt.Printf("Healing workflow failed: %v\n", err)
		}
	}()
}
