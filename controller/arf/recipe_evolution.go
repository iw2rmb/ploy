package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RecipeEvolution provides automatic recipe improvement based on failure analysis
type RecipeEvolution interface {
	AnalyzeFailure(ctx context.Context, failure TransformationFailure) (*FailureAnalysis, error)
	EvolveRecipe(ctx context.Context, recipe Recipe, analysis FailureAnalysis) (*Recipe, error)
	ValidateEvolution(ctx context.Context, original, evolved Recipe) (*ValidationResult, error)
	RollbackRecipe(ctx context.Context, recipeID string, version int) error
}

// ErrorType categorizes different types of transformation failures
type ErrorType string

const (
	ErrorRecipeMismatch       ErrorType = "recipe_mismatch"
	ErrorCompilationFailure   ErrorType = "compilation_failure"
	ErrorSemanticChange       ErrorType = "semantic_change"
	ErrorIncompleteTransform  ErrorType = "incomplete_transformation"
	ErrorResourceExhaustion   ErrorType = "resource_exhaustion"
	ErrorTimeoutFailure       ErrorType = "timeout_failure"
	ErrorDependencyIssue      ErrorType = "dependency_issue"
	ErrorUnknown              ErrorType = "unknown"
)

// FailureAnalysis contains the analysis of a transformation failure
type FailureAnalysis struct {
	ErrorType        ErrorType             `json:"error_type"`
	RootCause        string                `json:"root_cause"`
	SuggestedFixes   []RecipeModification  `json:"suggested_fixes"`
	Confidence       float64               `json:"confidence"`
	SimilarPatterns  []FailurePattern      `json:"similar_patterns"`
	AffectedFiles    []string              `json:"affected_files"`
	ContextInfo      map[string]interface{} `json:"context_info"`
	AnalysisTime     time.Time             `json:"analysis_time"`
}

// RecipeModification describes a specific change to make to a recipe
type RecipeModification struct {
	Type          ModificationType `json:"type"`
	Target        string           `json:"target"`
	Change        string           `json:"change"`
	Justification string           `json:"justification"`
	Priority      int              `json:"priority"`
	RiskLevel     RiskLevel        `json:"risk_level"`
}

// ModificationType defines the kind of modification to apply
type ModificationType string

const (
	ModificationAddRule       ModificationType = "add_rule"
	ModificationModifyRule    ModificationType = "modify_rule"
	ModificationRemoveRule    ModificationType = "remove_rule"
	ModificationAddCondition  ModificationType = "add_condition"
	ModificationAddException  ModificationType = "add_exception"
	ModificationAdjustPattern ModificationType = "adjust_pattern"
	ModificationExtendScope   ModificationType = "extend_scope"
	ModificationReduceScope   ModificationType = "reduce_scope"
)


// Note: FailurePattern type is defined in learning_system.go

// TransformationFailure contains details about a failed transformation
type TransformationFailure struct {
	RecipeID       string                 `json:"recipe_id"`
	ErrorMessage   string                 `json:"error_message"`
	StackTrace     string                 `json:"stack_trace,omitempty"`
	FailedFiles    []string               `json:"failed_files"`
	Codebase       Codebase               `json:"codebase"`
	Context        map[string]interface{} `json:"context"`
	FailureTime    time.Time              `json:"failure_time"`
	OperationLogs  []string               `json:"operation_logs,omitempty"`
}

// ValidationResult contains the results of recipe evolution validation
type ValidationResult struct {
	Valid           bool                   `json:"valid"`
	SafetyScore     float64                `json:"safety_score"`
	Warnings        []string               `json:"warnings"`
	CriticalIssues  []string               `json:"critical_issues"`
	TestResults     []ValidationTest       `json:"test_results"`
	RecommendAction ValidationAction       `json:"recommend_action"`
}

// ValidationTest represents a specific validation check
type ValidationTest struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	Details     string    `json:"details,omitempty"`
	Runtime     time.Duration `json:"runtime"`
}

// ValidationAction recommends what to do with the evolved recipe
type ValidationAction string

const (
	ActionApprove      ValidationAction = "approve"
	ActionReject       ValidationAction = "reject"
	ActionRequireReview ValidationAction = "require_review"
	ActionRunTests     ValidationAction = "run_tests"
)

// RecipeVersion tracks recipe evolution history
type RecipeVersion struct {
	Version       int                    `json:"version"`
	Recipe        Recipe                 `json:"recipe"`
	Changes       []RecipeModification   `json:"changes"`
	Reason        string                 `json:"reason"`
	CreatedAt     time.Time              `json:"created_at"`
	CreatedBy     string                 `json:"created_by"`
	Rollbackable  bool                   `json:"rollbackable"`
	TestResults   *ValidationResult      `json:"test_results,omitempty"`
}

// convertStringMap converts map[string]string to map[string]interface{}
func convertStringMap(stringMap map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range stringMap {
		result[k] = v
	}
	return result
}

// DefaultRecipeEvolution implements the RecipeEvolution interface
type DefaultRecipeEvolution struct {
	catalog        RecipeCatalog
	patternDB      ErrorPatternDB
	validator      RecipeValidator
	versioning     RecipeVersioning
	config         RecipeEvolutionConfig
}

// RecipeEvolutionConfig configures the recipe evolution behavior
type RecipeEvolutionConfig struct {
	MaxEvolutionDepth     int     `yaml:"max_evolution_depth"`
	MinConfidenceRequired float64 `yaml:"min_confidence_required"`
	EnableAutoApproval    bool    `yaml:"enable_auto_approval"`
	AutoApprovalThreshold float64 `yaml:"auto_approval_threshold"`
	RetainVersionHistory  int     `yaml:"retain_version_history"`
	EnablePatternLearning bool    `yaml:"enable_pattern_learning"`
}

// NewRecipeEvolution creates a new recipe evolution system
func NewRecipeEvolution(catalog RecipeCatalog, patternDB ErrorPatternDB, validator RecipeValidator, versioning RecipeVersioning) RecipeEvolution {
	config := RecipeEvolutionConfig{
		MaxEvolutionDepth:     5,
		MinConfidenceRequired: 0.7,
		EnableAutoApproval:    false,
		AutoApprovalThreshold: 0.9,
		RetainVersionHistory:  10,
		EnablePatternLearning: true,
	}

	return &DefaultRecipeEvolution{
		catalog:    catalog,
		patternDB:  patternDB,
		validator:  validator,
		versioning: versioning,
		config:     config,
	}
}

// AnalyzeFailure performs comprehensive analysis of a transformation failure
func (re *DefaultRecipeEvolution) AnalyzeFailure(ctx context.Context, failure TransformationFailure) (*FailureAnalysis, error) {
	analysis := &FailureAnalysis{
		ErrorType:     re.classifyError(failure),
		RootCause:     re.identifyRootCause(failure),
		ContextInfo:   failure.Context,
		AffectedFiles: failure.FailedFiles,
		AnalysisTime:  time.Now(),
	}

	// Find similar patterns if pattern learning is enabled
	if re.config.EnablePatternLearning && re.patternDB != nil {
		patterns, err := re.findSimilarPatterns(ctx, failure)
		if err == nil {
			analysis.SimilarPatterns = patterns
		}
	}

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

// findSimilarPatterns searches for similar failure patterns in the database
func (re *DefaultRecipeEvolution) findSimilarPatterns(ctx context.Context, failure TransformationFailure) ([]FailurePattern, error) {
	if re.patternDB == nil {
		return nil, fmt.Errorf("pattern database not available")
	}

	// Create a transformation error for pattern matching
	transformError := TransformationError{
		Message:     failure.ErrorMessage,
		Type:        string(re.classifyError(failure)),
		Recoverable: false,
	}

	// Find similar patterns with 0.7 similarity threshold
	errorPatterns, err := re.patternDB.FindSimilarPatterns(ctx, transformError, 0.7)
	if err != nil {
		return nil, err
	}

	// Convert to FailurePattern format
	patterns := make([]FailurePattern, 0, len(errorPatterns))
	for _, ep := range errorPatterns {
		pattern := FailurePattern{
			Signature:       ep.Signature,
			Frequency:       int32(ep.Occurrences),
			FailureRate:     float64(ep.Occurrences) / 100.0,
			CommonErrors:    []string{ep.ErrorMessage},
			ContextFactors:  []string{"environment_specific"},
			Mitigations:     []string{"review_configuration"},
		}

		// Find successful fix if available
		for _, solution := range ep.Solutions {
			if solution.Success {
				pattern.Mitigations[0] = solution.Description
				break
			}
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

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

// EvolveRecipe applies the suggested modifications to create an evolved recipe
func (re *DefaultRecipeEvolution) EvolveRecipe(ctx context.Context, recipe Recipe, analysis FailureAnalysis) (*Recipe, error) {
	if analysis.Confidence < re.config.MinConfidenceRequired {
		return nil, fmt.Errorf("analysis confidence %.2f below required threshold %.2f", 
			analysis.Confidence, re.config.MinConfidenceRequired)
	}

	// Create a copy of the original recipe
	evolved := recipe
	evolved.Version = recipe.Version + ".evolved"
	evolved.Description += " (auto-evolved based on failure analysis)"

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

	// Update recipe metadata
	if evolved.Options == nil {
		evolved.Options = make(map[string]string)
	}
	// Convert evolution notes to JSON string for storage
	notesJSON, _ := json.Marshal(evolutionNotes)
	evolved.Options["evolution_notes"] = string(notesJSON)
	evolved.Options["evolved_from"] = recipe.ID
	evolved.Options["evolution_time"] = time.Now().Format(time.RFC3339)
	evolved.Options["analysis_confidence"] = fmt.Sprintf("%.3f", analysis.Confidence)

	// Adjust confidence based on evolution success
	evolved.Confidence = recipe.Confidence * analysis.Confidence

	return &evolved, nil
}

// applyAddRule adds a new rule to the recipe
func (re *DefaultRecipeEvolution) applyAddRule(recipe Recipe, mod RecipeModification) Recipe {
	// In a real implementation, this would modify the OpenRewrite recipe YAML/configuration
	// For now, we add it to the options as metadata
	if recipe.Options == nil {
		recipe.Options = make(map[string]string)
	}
	
	// Get existing rules and append new one
	existingRules := []string{}
	if rulesJSON, exists := recipe.Options["additional_rules"]; exists {
		json.Unmarshal([]byte(rulesJSON), &existingRules)
	}
	existingRules = append(existingRules, mod.Change)
	
	// Store as JSON string
	rulesJSON, _ := json.Marshal(existingRules)
	recipe.Options["additional_rules"] = string(rulesJSON)
	
	return recipe
}

// applyModifyRule modifies an existing rule in the recipe
func (re *DefaultRecipeEvolution) applyModifyRule(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "rule_modifications", 
		fmt.Sprintf("Modified %s: %s", mod.Target, mod.Change))
}

// applyAddCondition adds a condition to the recipe
func (re *DefaultRecipeEvolution) applyAddCondition(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "additional_conditions", mod.Change)
}

// applyAddException adds an exception to the recipe
func (re *DefaultRecipeEvolution) applyAddException(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "exceptions", mod.Change)
}

// applyAdjustPattern adjusts pattern matching in the recipe
func (re *DefaultRecipeEvolution) applyAdjustPattern(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "pattern_adjustments", mod.Change)
}

// applyExtendScope extends the scope of the recipe
func (re *DefaultRecipeEvolution) applyExtendScope(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "scope_extensions", mod.Change)
}

// applyReduceScope reduces the scope of the recipe
func (re *DefaultRecipeEvolution) applyReduceScope(recipe Recipe, mod RecipeModification) Recipe {
	return re.appendToOptionArray(recipe, "scope_reductions", mod.Change)
}

// appendToOptionArray is a helper to append values to JSON array stored in options
func (re *DefaultRecipeEvolution) appendToOptionArray(recipe Recipe, key, value string) Recipe {
	if recipe.Options == nil {
		recipe.Options = make(map[string]string)
	}
	
	// Get existing array and append new value
	existing := []string{}
	if arrayJSON, exists := recipe.Options[key]; exists {
		json.Unmarshal([]byte(arrayJSON), &existing)
	}
	existing = append(existing, value)
	
	// Store as JSON string
	arrayJSON, _ := json.Marshal(existing)
	recipe.Options[key] = string(arrayJSON)
	
	return recipe
}

// ValidateEvolution validates that an evolved recipe is safe to use
func (re *DefaultRecipeEvolution) ValidateEvolution(ctx context.Context, original, evolved Recipe) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:       true,
		SafetyScore: 1.0,
		Warnings:    []string{},
		CriticalIssues: []string{},
		TestResults: []ValidationTest{},
		RecommendAction: ActionApprove,
	}

	// Validate confidence change
	confidenceTest := ValidationTest{
		Name:        "confidence_validation",
		Description: "Check if evolved recipe maintains reasonable confidence",
		Runtime:     10 * time.Millisecond,
	}

	if evolved.Confidence < 0.5 {
		confidenceTest.Status = "failed"
		confidenceTest.Details = fmt.Sprintf("Confidence too low: %.2f", evolved.Confidence)
		result.CriticalIssues = append(result.CriticalIssues, confidenceTest.Details)
		result.Valid = false
		result.SafetyScore -= 0.3
	} else if evolved.Confidence < 0.7 {
		confidenceTest.Status = "warning"
		confidenceTest.Details = fmt.Sprintf("Low confidence: %.2f", evolved.Confidence)
		result.Warnings = append(result.Warnings, confidenceTest.Details)
		result.SafetyScore -= 0.1
	} else {
		confidenceTest.Status = "passed"
		confidenceTest.Details = fmt.Sprintf("Confidence acceptable: %.2f", evolved.Confidence)
	}

	result.TestResults = append(result.TestResults, confidenceTest)

	// Validate evolution options
	optionsTest := ValidationTest{
		Name:        "options_validation",
		Description: "Validate evolved recipe options",
		Runtime:     5 * time.Millisecond,
	}

	if evolved.Options != nil {
		if notesJSON, exists := evolved.Options["evolution_notes"]; exists {
			// Parse the JSON string to get the actual notes
			var noteSlice []string
			if err := json.Unmarshal([]byte(notesJSON), &noteSlice); err == nil && len(noteSlice) > 0 {
				optionsTest.Status = "passed"
				optionsTest.Details = fmt.Sprintf("Evolution notes: %d modifications", len(noteSlice))
			} else {
				optionsTest.Status = "warning"
				optionsTest.Details = "Evolution notes format invalid"
				result.Warnings = append(result.Warnings, optionsTest.Details)
			}
		} else {
			optionsTest.Status = "warning"
			optionsTest.Details = "No evolution notes found"
			result.Warnings = append(result.Warnings, optionsTest.Details)
		}
	} else {
		optionsTest.Status = "failed"
		optionsTest.Details = "No options found in evolved recipe"
		result.CriticalIssues = append(result.CriticalIssues, optionsTest.Details)
		result.SafetyScore -= 0.2
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

// RollbackRecipe rolls back a recipe to a previous version
func (re *DefaultRecipeEvolution) RollbackRecipe(ctx context.Context, recipeID string, version int) error {
	if re.versioning == nil {
		return fmt.Errorf("recipe versioning not available")
	}

	// Get the specific version
	recipeVersion, err := re.versioning.GetVersion(ctx, recipeID, version)
	if err != nil {
		return fmt.Errorf("failed to get recipe version %d: %w", version, err)
	}

	if !recipeVersion.Rollbackable {
		return fmt.Errorf("recipe version %d is not rollbackable", version)
	}

	// Store the current version as a rollback point
	currentRecipe, err := re.catalog.GetRecipe(ctx, recipeID)
	if err != nil {
		return fmt.Errorf("failed to get current recipe: %w", err)
	}

	rollbackVersion := RecipeVersion{
		Version:      re.versioning.GetNextVersion(ctx, recipeID),
		Recipe:       *currentRecipe,
		Changes:      []RecipeModification{},
		Reason:       fmt.Sprintf("Rollback to version %d", version),
		CreatedAt:    time.Now(),
		CreatedBy:    "system",
		Rollbackable: true,
	}

	// Store the rollback version
	if err := re.versioning.StoreVersion(ctx, rollbackVersion); err != nil {
		return fmt.Errorf("failed to store rollback version: %w", err)
	}

	// Update the catalog with the rolled-back recipe
	return re.catalog.UpdateRecipe(ctx, recipeVersion.Recipe)
}

// ErrorPattern represents a stored error pattern in the database
type ErrorPattern struct {
	ID              string              `json:"id"`
	Signature       string              `json:"signature"`
	ErrorType       string              `json:"error_type"`
	Context         ErrorContext        `json:"context"`
	Solutions       []Solution          `json:"solutions"`
	Effectiveness   float64             `json:"effectiveness"`
	Occurrences     int                 `json:"occurrences"`
	LastSeen        time.Time           `json:"last_seen"`
	Embedding       []float64           `json:"embedding"`
}


// Solution represents a successful fix for an error pattern
type Solution struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Success     bool      `json:"success"`
	Confidence  float64   `json:"confidence"`
	AppliedAt   time.Time `json:"applied_at"`
}

// Placeholder interfaces for dependencies
type ErrorPatternDB interface {
	FindSimilarPatterns(ctx context.Context, error TransformationError, threshold float64) ([]ErrorPattern, error)
}

type RecipeValidator interface {
	ValidateRecipe(ctx context.Context, recipe Recipe) error
}

type RecipeVersioning interface {
	StoreVersion(ctx context.Context, version RecipeVersion) error
	GetVersion(ctx context.Context, recipeID string, version int) (*RecipeVersion, error)
	GetNextVersion(ctx context.Context, recipeID string) int
	ListVersions(ctx context.Context, recipeID string) ([]RecipeVersion, error)
}