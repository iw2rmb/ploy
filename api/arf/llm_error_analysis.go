package arf

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EnhancedLLMAnalyzer provides sophisticated LLM-based error analysis for healing workflows
type EnhancedLLMAnalyzer struct {
	llmGenerator    LLMRecipeGenerator
	llmDispatcher   *LLMDispatcher
	cache           map[string]*LLMAnalysisResult // Simple cache for similar errors
	cacheMutex      sync.RWMutex
	cacheExpiry     time.Duration
	costTracker     *LLMCostTracker         // Track costs and optimize usage
	metricsExporter *HealingMetricsExporter // Prometheus metrics
}

// EnhancedErrorPattern represents a pattern for error detection in LLM analysis
type EnhancedErrorPattern struct {
	Pattern    *regexp.Regexp
	Type       string
	Confidence float64
	Language   string
	Extractor  func([]string) string // Extract relevant info from error
}

// ExtractErrorContext is a standalone function that extracts context from error messages
func ExtractErrorContext(errors []string, language string) ErrorContext {
	context := ErrorContext{
		ErrorMessage: strings.Join(errors, "\n"),
		ErrorType:    "compilation",
		ErrorDetails: make(map[string]string),
		Timestamp:    time.Now(),
	}

	// Detect error type
	errorText := strings.ToLower(strings.Join(errors, " "))
	if strings.Contains(errorText, "test") && (strings.Contains(errorText, "fail") || strings.Contains(errorText, "assertion")) {
		context.ErrorType = "test"
	} else if strings.Contains(errorText, "import") || strings.Contains(errorText, "module") {
		context.ErrorType = "import"
	} else if strings.Contains(errorText, "dependency") || strings.Contains(errorText, "version") {
		context.ErrorType = "dependency"
	}

	// Extract source file and line number
	for _, err := range errors {
		lines := strings.Split(err, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Look for file:line:column pattern
			if strings.Contains(trimmed, ".go:") || strings.Contains(trimmed, ".java:") ||
				strings.Contains(trimmed, ".py:") || strings.Contains(trimmed, ".js:") ||
				strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "/") {

				// Handle Go-specific pattern "main.go:5:2: error"
				if strings.Contains(trimmed, ".go:") {
					// Find the .go: pattern
					goIndex := strings.Index(trimmed, ".go:")
					if goIndex != -1 {
						// Find start of filename (could be just "main.go" or "./main.go")
						start := 0
						for i := goIndex - 1; i >= 0; i-- {
							if trimmed[i] == ' ' || trimmed[i] == '\t' {
								start = i + 1
								break
							}
						}
						filePath := trimmed[start : goIndex+3] // Include ".go"
						context.SourceFile = filePath

						// Extract line number
						remainder := trimmed[goIndex+4:] // After ".go:"
						parts := strings.Split(remainder, ":")
						if len(parts) > 0 {
							lineNum := strings.TrimSpace(parts[0])
							if _, err := strconv.Atoi(lineNum); err == nil {
								context.ErrorDetails["line_number"] = lineNum
							}
						}
						break
					}
				} else {
					// Generic file:line pattern
					parts := strings.Split(trimmed, ":")
					if len(parts) >= 2 {
						context.SourceFile = parts[0]
						// Try to parse line number
						lineNum := strings.TrimSpace(parts[1])
						if _, err := strconv.Atoi(lineNum); err == nil {
							context.ErrorDetails["line_number"] = lineNum
						}
						break
					}
				}
			}
		}
		if context.SourceFile != "" {
			break
		}
	}

	// Extract stack trace for runtime errors
	var stackTrace []string
	for _, err := range errors {
		if strings.Contains(err, "\tat ") || strings.Contains(err, "goroutine") {
			lines := strings.Split(err, "\n")
			for _, line := range lines {
				if strings.Contains(line, "\tat ") || strings.Contains(line, ".go:") || strings.Contains(line, ".java:") {
					stackTrace = append(stackTrace, strings.TrimSpace(line))
				}
			}
		}
	}
	if len(stackTrace) > 0 {
		context.StackTrace = stackTrace
		if context.ErrorType == "compilation" {
			context.ErrorType = "runtime"
		}
	}

	return context
}

// NewEnhancedLLMAnalyzer creates a new enhanced LLM analyzer
func NewEnhancedLLMAnalyzer(generator LLMRecipeGenerator, dispatcher *LLMDispatcher) *EnhancedLLMAnalyzer {
	// Create default budget config
	budgetConfig := &LLMBudgetConfig{
		Enabled:               true,
		MaxCostPerTransform:   5.0,   // $5 per transformation
		MaxCostPerHour:        25.0,  // $25 per hour
		MaxCostPerDay:         250.0, // $250 per day
		AlertThresholdPercent: 80.0,
		FallbackModel:         "gpt-3.5-turbo",
		BlockOnExceed:         false, // Don't block, just alert
	}

	return &EnhancedLLMAnalyzer{
		llmGenerator:  generator,
		llmDispatcher: dispatcher,
		cache:         make(map[string]*LLMAnalysisResult),
		cacheExpiry:   30 * time.Minute,
		costTracker:   NewLLMCostTracker(budgetConfig),
	}
}

// SetMetricsExporter sets the Prometheus metrics exporter
func (a *EnhancedLLMAnalyzer) SetMetricsExporter(exporter *HealingMetricsExporter) {
	a.metricsExporter = exporter
}

// AnalyzeErrors performs comprehensive LLM-based error analysis
func (a *EnhancedLLMAnalyzer) AnalyzeErrors(ctx context.Context, errors []string, language string) (*LLMAnalysisResult, error) {
	// Check cache first
	cacheKey := a.generateCacheKey(errors)
	if cached := a.getFromCache(cacheKey); cached != nil {
		// Record cache hit in cost tracker
		if a.costTracker != nil {
			a.costTracker.RecordUsage(ctx, LLMUsageRecord{
				Model:        "cache",
				InputTokens:  0,
				OutputTokens: 0,
				TotalCost:    0,
				CacheHit:     true,
				TransformID:  ctx.Value("transformID").(string),
			})
		}
		return cached, nil
	}

	// Extract error context
	errorContext := a.extractErrorContext(errors)

	// Check if we have LLM cost tracking cache
	prompt := a.generateHealingPrompt(errorContext, language)
	modelToUse := "gpt-4-turbo" // Default model

	if a.costTracker != nil {
		// Check for cached LLM response
		if cachedEntry, found := a.costTracker.GetCachedResponse(prompt, modelToUse); found {
			// Parse cached response
			result := a.parseCachedResponse(cachedEntry.Response, errorContext)
			a.storeInCache(cacheKey, result)

			// Get transformation ID safely
			transformID := ""
			if ctx.Value("transformID") != nil {
				transformID = ctx.Value("transformID").(string)
			}

			// Log cache hit
			GetHealingLogger().WithFields(LogFields{
				"transformation_id": transformID,
				"model":             modelToUse,
				"cache_hit":         true,
			}).Debug("Using cached LLM response")

			// Log LLM cost with cache hit
			GetHealingLogger().LogLLMCost(transformID, modelToUse, 0, 0, 0, true)

			// Record Prometheus metrics for cache hit
			if a.metricsExporter != nil {
				a.metricsExporter.RecordLLMCall(modelToUse, true, 0)
			}

			// Record cache hit
			a.costTracker.RecordUsage(ctx, LLMUsageRecord{
				Model:        modelToUse,
				InputTokens:  0,
				OutputTokens: 0,
				TotalCost:    0,
				CacheHit:     true,
				TransformID:  transformID,
			})

			return result, nil
		}

		// Estimate tokens and check budget
		estimatedTokens := a.costTracker.EstimateTokens(prompt)
		allowed, reason, err := a.costTracker.CheckBudget(modelToUse, estimatedTokens)
		if err != nil {
			// Log error but continue
			GetHealingLogger().WithFields(LogFields{
				"model":            modelToUse,
				"estimated_tokens": estimatedTokens,
			}).Error("Budget check error", err)
		}

		if !allowed {
			// Switch to fallback model or pattern-based analysis
			GetHealingLogger().WithFields(LogFields{
				"model":            modelToUse,
				"estimated_tokens": estimatedTokens,
				"reason":           reason,
			}).Warn("LLM budget exceeded, using fallback pattern-based analysis")
			result := a.analyzeErrorsWithPattern(errors, language)
			a.storeInCache(cacheKey, result)
			return result, nil
		}

		// Suggest optimal model based on quality needs
		modelToUse = a.costTracker.SuggestOptimalModel(estimatedTokens, 0.7) // 70% quality priority
	}

	// Use LLM if available
	if a.llmGenerator != nil && a.llmGenerator.IsAvailable(ctx) {
		// Store prompt in error context metadata
		if errorContext.Metadata == nil {
			errorContext.Metadata = make(map[string]interface{})
		}
		errorContext.Metadata["healing_prompt"] = prompt
		errorContext.Metadata["model"] = modelToUse

		request := RecipeGenerationRequest{
			ErrorContext: errorContext,
			Language:     language,
		}

		// Log LLM analysis start
		transformID := ""
		if ctx.Value("transformID") != nil {
			transformID = ctx.Value("transformID").(string)
		}
		GetHealingLogger().WithFields(LogFields{
			"transformation_id": transformID,
			"model":             modelToUse,
			"error_type":        errorContext.ErrorType,
		}).Debug("Starting LLM error analysis")

		startTime := time.Now()
		recipe, err := a.llmGenerator.GenerateRecipe(ctx, request)
		duration := time.Since(startTime)

		if err == nil && recipe != nil {
			result := a.parseGeneratedRecipe(recipe, errorContext)
			a.storeInCache(cacheKey, result)

			// Track LLM usage and costs
			if a.costTracker != nil {
				// Estimate tokens from prompt and response
				inputTokens := a.costTracker.EstimateTokens(prompt)
				outputTokens := a.costTracker.EstimateTokens(recipe.Description + recipe.Explanation)

				// Calculate cost
				cost, _ := a.costTracker.CalculateCost(modelToUse, inputTokens, outputTokens)

				// Log LLM cost
				GetHealingLogger().LogLLMCost(transformID, modelToUse, inputTokens, outputTokens, cost, false)

				// Record Prometheus metrics for LLM call
				if a.metricsExporter != nil {
					a.metricsExporter.RecordLLMCall(modelToUse, false, cost)
				}

				// Record usage
				a.costTracker.RecordUsage(ctx, LLMUsageRecord{
					Model:        modelToUse,
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					TotalCost:    cost,
					Prompt:       prompt,
					Response:     recipe.Description,
					CacheHit:     false,
					TransformID:  transformID,
					Duration:     duration,
				})

				// Cache the response for future use
				a.costTracker.CacheResponse(prompt, recipe.Description, modelToUse, inputTokens+outputTokens, cost)
			}

			// Log successful analysis
			GetHealingLogger().LogLLMAnalysis(ctx, transformID, "", result)

			return result, nil
		} else if err != nil {
			// Log LLM error
			GetHealingLogger().WithFields(LogFields{
				"transformation_id": transformID,
				"model":             modelToUse,
				"duration_ms":       duration.Milliseconds(),
			}).Error("LLM recipe generation failed", err)

			if a.costTracker != nil {
				// Record error
				a.costTracker.RecordUsage(ctx, LLMUsageRecord{
					Model:       modelToUse,
					CacheHit:    false,
					TransformID: transformID,
					Duration:    duration,
					Error:       err.Error(),
				})
			}
		}
	}

	// Fallback to pattern-based analysis
	result := a.analyzeErrorsWithPattern(errors, language)
	a.storeInCache(cacheKey, result)
	return result, nil
}

// extractErrorContext extracts structured context from error messages
func (a *EnhancedLLMAnalyzer) extractErrorContext(errors []string) ErrorContext {
	// Use standalone function for consistency
	return ExtractErrorContext(errors, "")
}

// generateHealingPrompt creates an intelligent prompt for LLM analysis
func (a *EnhancedLLMAnalyzer) generateHealingPrompt(errorContext ErrorContext, language string) string {
	languageExpert := map[string]string{
		"java":       "Java",
		"python":     "Python",
		"go":         "Go",
		"javascript": "JavaScript",
		"typescript": "TypeScript",
		"cpp":        "C++",
		"c":          "C",
	}

	expert := languageExpert[strings.ToLower(language)]
	if expert == "" {
		expert = "software"
	}

	prompt := fmt.Sprintf(`You are an expert %s developer. Analyze the following %s error and provide a fix.

Error Type: %s
Source File: %s
Error Message:
%s

Please provide:
1. Root cause analysis
2. Suggested fix with code
3. Alternative solutions
4. Risk assessment (low/medium/high)
`, expert, errorContext.ErrorType, errorContext.ErrorType, errorContext.SourceFile, errorContext.ErrorMessage)

	// Add specific instructions based on error type
	switch errorContext.ErrorType {
	case "compilation":
		prompt += "5. Required dependencies or imports\n"
	case "test":
		prompt += "5. Whether to fix the code or update the test\n"
	case "import":
		prompt += "5. Correct import statement or package installation command\n"
	case "dependency":
		prompt += "5. Dependency version compatibility information\n"
	}

	prompt += "\nFormat your response as JSON with fields: suggested_fix, alternative_fixes, risk_assessment, confidence_score (0-1)"

	return prompt
}

// analyzeErrorsWithPattern performs pattern-based error analysis as fallback
func (a *EnhancedLLMAnalyzer) analyzeErrorsWithPattern(errors []string, language string) *LLMAnalysisResult {
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

// parseCachedResponse converts cached LLM response to LLMAnalysisResult
func (a *EnhancedLLMAnalyzer) parseCachedResponse(response string, errorContext ErrorContext) *LLMAnalysisResult {
	return &LLMAnalysisResult{
		ErrorType:        errorContext.ErrorType,
		SuggestedFix:     response,
		Confidence:       0.85, // High confidence for cached responses
		AlternativeFixes: []string{},
		RiskAssessment:   "low", // Cached responses are proven safe
	}
}

// parseGeneratedRecipe converts LLM response to LLMAnalysisResult
func (a *EnhancedLLMAnalyzer) parseGeneratedRecipe(recipe *GeneratedRecipe, errorContext ErrorContext) *LLMAnalysisResult {
	result := &LLMAnalysisResult{
		ErrorType:        errorContext.ErrorType,
		Confidence:       0.8, // Default confidence
		AlternativeFixes: []string{},
		RiskAssessment:   "medium",
	}

	// Parse the recipe content from LLM metadata
	if recipe.LLMMetadata != nil {
		// Try to extract suggested fix from LLM metadata
		if suggestedFix, ok := recipe.LLMMetadata["suggested_fix"].(string); ok {
			result.SuggestedFix = suggestedFix
		}
		if alternativeFixes, ok := recipe.LLMMetadata["alternative_fixes"].([]interface{}); ok {
			for _, fix := range alternativeFixes {
				if fixStr, ok := fix.(string); ok {
					result.AlternativeFixes = append(result.AlternativeFixes, fixStr)
				}
			}
		}
		if riskAssessment, ok := recipe.LLMMetadata["risk_assessment"].(string); ok {
			result.RiskAssessment = riskAssessment
		}
		if confidenceScore, ok := recipe.LLMMetadata["confidence_score"].(float64); ok {
			result.Confidence = confidenceScore
		}
	}

	// Use recipe description as fallback for suggested fix
	if result.SuggestedFix == "" && recipe.Description != "" {
		result.SuggestedFix = recipe.Description
		result.Confidence = recipe.Confidence
	}

	// Add recipe name to alternatives if available
	if recipe.Name != "" {
		result.AlternativeFixes = append(result.AlternativeFixes,
			fmt.Sprintf("Apply OpenRewrite recipe: %s", recipe.Name))
	}

	return result
}

// convertToOpenRewriteRecipe converts analysis to OpenRewrite recipe
func (a *EnhancedLLMAnalyzer) convertToOpenRewriteRecipe(analysis *LLMAnalysisResult, language string) (string, map[string]interface{}) {
	metadata := make(map[string]interface{})

	// Map common fixes to OpenRewrite recipes
	suggestedFix := strings.ToLower(analysis.SuggestedFix)

	switch language {
	case "java":
		if strings.Contains(suggestedFix, "add import") {
			// Extract import statement
			importPattern := regexp.MustCompile(`import\s+([\w\.]+);?`)
			if matches := importPattern.FindStringSubmatch(analysis.SuggestedFix); len(matches) > 1 {
				metadata["type"] = matches[1]
				metadata["onlyIfUsed"] = true
				return "org.openrewrite.java.AddImport", metadata
			}
		} else if strings.Contains(suggestedFix, "remove unused") {
			return "org.openrewrite.java.RemoveUnusedImports", metadata
		} else if analysis.ErrorType == "compilation" {
			return "org.openrewrite.java.cleanup.UnnecessaryThrows", metadata
		}

	case "python":
		if strings.Contains(suggestedFix, "remove unused import") {
			// Extract module name
			modulePattern := regexp.MustCompile(`import\s+(\w+)`)
			if matches := modulePattern.FindStringSubmatch(analysis.SuggestedFix); len(matches) > 1 {
				metadata["module"] = matches[1]
			}
			return "org.openrewrite.python.cleanup.RemoveUnusedImports", metadata
		}

	case "go":
		if strings.Contains(suggestedFix, "gofmt") || strings.Contains(suggestedFix, "format") {
			return "org.openrewrite.go.format", metadata
		}
	}

	// Default generic recipe
	return "org.openrewrite.text.Find", metadata
}

// BatchAnalyzeErrors analyzes multiple error sets in batch
func (a *EnhancedLLMAnalyzer) BatchAnalyzeErrors(ctx context.Context, errorSets [][]string, language string) []*LLMAnalysisResult {
	results := make([]*LLMAnalysisResult, len(errorSets))
	var wg sync.WaitGroup

	for i, errors := range errorSets {
		wg.Add(1)
		go func(index int, errs []string) {
			defer wg.Done()

			result, err := a.AnalyzeErrors(ctx, errs, language)
			if err != nil {
				// Fallback to pattern analysis
				result = a.analyzeErrorsWithPattern(errs, language)
			}
			results[index] = result
		}(i, errors)
	}

	wg.Wait()
	return results
}

// Cache management functions
func (a *EnhancedLLMAnalyzer) generateCacheKey(errors []string) string {
	// Simple hash of error messages
	return fmt.Sprintf("%v", errors)
}

func (a *EnhancedLLMAnalyzer) getFromCache(key string) *LLMAnalysisResult {
	a.cacheMutex.RLock()
	defer a.cacheMutex.RUnlock()
	return a.cache[key]
}

func (a *EnhancedLLMAnalyzer) storeInCache(key string, result *LLMAnalysisResult) {
	a.cacheMutex.Lock()
	defer a.cacheMutex.Unlock()
	a.cache[key] = result

	// Simple cache cleanup - in production, use proper TTL
	if len(a.cache) > 100 {
		// Clear oldest entries
		for k := range a.cache {
			delete(a.cache, k)
			if len(a.cache) <= 50 {
				break
			}
		}
	}
}

// AnalyzeAndSuggestHealing is the main entry point for healing workflow integration
func (a *EnhancedLLMAnalyzer) AnalyzeAndSuggestHealing(ctx context.Context, errors []string, language string, sandboxID string) (*HealingSuggestion, error) {
	// Analyze errors
	analysis, err := a.AnalyzeErrors(ctx, errors, language)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze errors: %w", err)
	}

	// Convert to OpenRewrite recipe
	recipeName, recipeMetadata := a.convertToOpenRewriteRecipe(analysis, language)

	// Create healing suggestion
	suggestion := &HealingSuggestion{
		Analysis:        analysis,
		RecipeName:      recipeName,
		RecipeMetadata:  recipeMetadata,
		SandboxID:       sandboxID,
		Language:        language,
		Confidence:      analysis.Confidence,
		EstimatedImpact: a.estimateImpact(analysis),
		Prerequisites:   a.determinePrerequisites(analysis, language),
	}

	return suggestion, nil
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

// estimateImpact estimates the impact of applying the healing suggestion
func (a *EnhancedLLMAnalyzer) estimateImpact(analysis *LLMAnalysisResult) string {
	if analysis.RiskAssessment == "high" {
		return "major"
	} else if analysis.RiskAssessment == "medium" {
		return "moderate"
	}
	return "minor"
}

// determinePrerequisites determines what needs to be in place before applying the fix
func (a *EnhancedLLMAnalyzer) determinePrerequisites(analysis *LLMAnalysisResult, language string) []string {
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

// GetCostMetrics returns the current LLM usage metrics
func (a *EnhancedLLMAnalyzer) GetCostMetrics() *LLMUsageMetrics {
	if a.costTracker != nil {
		return a.costTracker.GetMetrics()
	}
	return nil
}

// GetTransformationLLMCost returns the total LLM cost for a specific transformation
func (a *EnhancedLLMAnalyzer) GetTransformationLLMCost(transformID string) float64 {
	if a.costTracker != nil {
		return a.costTracker.GetTransformationCost(transformID)
	}
	return 0
}

// RegisterCostAlertHandler registers a handler for budget alerts
func (a *EnhancedLLMAnalyzer) RegisterCostAlertHandler(handler func(BudgetAlert)) {
	if a.costTracker != nil {
		a.costTracker.RegisterAlertCallback(handler)
	}
}
