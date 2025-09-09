package arf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EnhancedLLMAnalyzer provides sophisticated LLM-based error analysis for healing workflows
type EnhancedLLMAnalyzer struct {
	llmGenerator       LLMRecipeGenerator
	llmDispatcher      *LLMDispatcher
	patternAnalyzer    *PatternAnalyzer
	analysisCache      *AnalysisCache
	healingSuggestions *HealingSuggestionService
	costTracker        *LLMCostTracker         // Track costs and optimize usage
	metricsExporter    *HealingMetricsExporter // Prometheus metrics
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
		llmGenerator:       generator,
		llmDispatcher:      dispatcher,
		patternAnalyzer:    NewPatternAnalyzer(),
		analysisCache:      NewAnalysisCache(30 * time.Minute),
		healingSuggestions: NewHealingSuggestionService(),
		costTracker:        NewLLMCostTracker(budgetConfig),
	}
}

// SetMetricsExporter sets the Prometheus metrics exporter
func (a *EnhancedLLMAnalyzer) SetMetricsExporter(exporter *HealingMetricsExporter) {
	a.metricsExporter = exporter
}

// AnalyzeErrors performs comprehensive LLM-based error analysis
func (a *EnhancedLLMAnalyzer) AnalyzeErrors(ctx context.Context, errors []string, language string) (*LLMAnalysisResult, error) {
	// Check cache first
	cacheKey := a.analysisCache.GenerateCacheKey(errors)
	if cached := a.analysisCache.GetFromCache(cacheKey); cached != nil {
		// Record cache hit in cost tracker
		if a.costTracker != nil {
			a.costTracker.RecordUsage(ctx, LLMUsageRecord{
				Model:        "cache",
				InputTokens:  0,
				OutputTokens: 0,
				TotalCost:    0,
				CacheHit:     true,
				TransformID:  a.getTransformID(ctx),
			})
		}
		return cached, nil
	}

	// Extract error context using standalone function
	errorContext := ExtractErrorContext(errors, language)

	// Check if we have LLM cost tracking cache
	prompt := a.generateHealingPrompt(errorContext, language)
	modelToUse := "gpt-4-turbo" // Default model

	if a.costTracker != nil {
		// Check for cached LLM response
		if cachedEntry, found := a.costTracker.GetCachedResponse(prompt, modelToUse); found {
			// Parse cached response
			result := a.parseCachedResponse(cachedEntry.Response, errorContext)
			a.analysisCache.StoreInCache(cacheKey, result)

			// Get transformation ID safely
			transformID := a.getTransformID(ctx)

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
			result := a.patternAnalyzer.AnalyzeErrors(errors, language)
			a.analysisCache.StoreInCache(cacheKey, result)
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
		transformID := a.getTransformID(ctx)
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
			a.analysisCache.StoreInCache(cacheKey, result)

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
	result := a.patternAnalyzer.AnalyzeErrors(errors, language)
	a.analysisCache.StoreInCache(cacheKey, result)
	return result, nil
}

// getTransformID safely extracts transform ID from context
func (a *EnhancedLLMAnalyzer) getTransformID(ctx context.Context) string {
	if ctx.Value("transformID") != nil {
		return ctx.Value("transformID").(string)
	}
	return ""
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
				result = a.patternAnalyzer.AnalyzeErrors(errs, language)
			}
			results[index] = result
		}(i, errors)
	}

	wg.Wait()
	return results
}

// AnalyzeAndSuggestHealing is the main entry point for healing workflow integration
func (a *EnhancedLLMAnalyzer) AnalyzeAndSuggestHealing(ctx context.Context, errors []string, language string, sandboxID string) (*HealingSuggestion, error) {
	// Analyze errors
	analysis, err := a.AnalyzeErrors(ctx, errors, language)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze errors: %w", err)
	}

	// Use healing suggestion service to create suggestion
	return a.healingSuggestions.CreateHealingSuggestion(ctx, analysis, language, sandboxID)
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
