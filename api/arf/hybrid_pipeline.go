package arf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// HybridPipeline defines the interface for combining OpenRewrite and LLM transformations
type HybridPipeline interface {
	ExecuteHybridTransformation(ctx context.Context, request HybridRequest) (*HybridResult, error)
	SelectOptimalStrategy(ctx context.Context, analysis ComplexityAnalysis) (*TransformationStrategy, error)
	EnhanceWithLLM(ctx context.Context, baseResult TransformationResult) (*EnhancedResult, error)
}

// HybridRequest contains all information needed for hybrid transformation
type HybridRequest struct {
	Repository      Repository              `json:"repository"`
	PrimaryRecipe   *models.Recipe          `json:"primary_recipe"`
	Context         TransformationContext   `json:"context"`
	EnhancementMode EnhancementMode         `json:"enhancement_mode"`
	Confidence      ConfidenceThresholds    `json:"confidence"`
}

// HybridResult contains the results of hybrid transformation
type HybridResult struct {
	TransformationResult
	PrimaryResult   *TransformationResult `json:"primary_result"`
	EnhancedResult  *EnhancedResult       `json:"enhanced_result"`
	Strategy        TransformationStrategy `json:"strategy"`
	TotalTime       time.Duration         `json:"total_time"`
	ConfidenceScore float64               `json:"confidence_score"`
}

// TransformationStrategy defines the approach for transformation
type TransformationStrategy struct {
	Primary     StrategyType    `json:"primary"`
	Enhancement StrategyType    `json:"enhancement"`
	Confidence  float64         `json:"confidence"`
	Reasoning   string          `json:"reasoning"`
	Fallbacks   []StrategyType  `json:"fallbacks"`
}

// StrategyType defines different transformation strategies
type StrategyType string

const (
	StrategyOpenRewriteOnly StrategyType = "openrewrite_only"
	StrategyLLMOnly         StrategyType = "llm_only"
	StrategyHybridSequential StrategyType = "hybrid_sequential"
	StrategyHybridParallel  StrategyType = "hybrid_parallel"
	StrategyTreeSitter      StrategyType = "tree_sitter"
	StrategyManualReview    StrategyType = "manual_review"
)

// EnhancementMode defines how LLM enhancement is applied
type EnhancementMode int

const (
	NoEnhancement EnhancementMode = iota
	PostProcessing
	ParallelValidation
	FullHybrid
)

// ConfidenceThresholds define minimum confidence levels for each strategy
type ConfidenceThresholds struct {
	MinOpenRewrite float64 `json:"min_openrewrite"`
	MinLLM         float64 `json:"min_llm"`
	MinHybrid      float64 `json:"min_hybrid"`
	RequiredBuild  float64 `json:"required_build"`
}

// TransformationContext provides context for transformation decisions
type TransformationContext struct {
	ErrorHistory        []ErrorContext        `json:"error_history"`
	PreviousAttempts    []TransformationResult `json:"previous_attempts"`
	TimeConstraints     TimeConstraints       `json:"time_constraints"`
	ResourceConstraints ResourceConstraints   `json:"resource_constraints"`
	QualityRequirements QualityRequirements   `json:"quality_requirements"`
}

// TimeConstraints define time limits for transformations
type TimeConstraints struct {
	MaxDuration    time.Duration `json:"max_duration"`
	PreferredSpeed string        `json:"preferred_speed"` // "fast", "balanced", "thorough"
}

// ResourceConstraints define resource limits
type ResourceConstraints struct {
	MaxMemory     int64 `json:"max_memory"`
	MaxCPU        int   `json:"max_cpu"`
	MaxTokens     int   `json:"max_tokens"`
	CostBudget    float64 `json:"cost_budget"`
}

// QualityRequirements define quality expectations
type QualityRequirements struct {
	MinConfidence     float64 `json:"min_confidence"`
	RequireTests      bool    `json:"require_tests"`
	RequireValidation bool    `json:"require_validation"`
	SafetyLevel       string  `json:"safety_level"` // "low", "medium", "high"
}

// EnhancedResult contains LLM-enhanced transformation results
type EnhancedResult struct {
	OriginalResult      TransformationResult  `json:"original_result"`
	EnhancedChanges     []CodeChange          `json:"enhanced_changes"`
	LLMSuggestions      []LLMSuggestion       `json:"llm_suggestions"`
	ConfidenceImprovement float64             `json:"confidence_improvement"`
	EnhancementTime     time.Duration         `json:"enhancement_time"`
	EnhancementMetadata map[string]interface{} `json:"enhancement_metadata"`
}

// LLMSuggestion represents an LLM-provided improvement suggestion
type LLMSuggestion struct {
	Type        string  `json:"type"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
	CodeChange  CodeChange `json:"code_change"`
	Reasoning   string  `json:"reasoning"`
}

// ComplexityAnalysis contains analysis of transformation complexity
type ComplexityAnalysis struct {
	OverallComplexity    float64                    `json:"overall_complexity"`
	FactorBreakdown     map[string]float64         `json:"factor_breakdown"`
	PredictedChallenges []PredictedChallenge       `json:"predicted_challenges"`
	RecommendedApproach RecommendedApproach        `json:"recommended_approach"`
}

// PredictedChallenge represents a potential difficulty in transformation
type PredictedChallenge struct {
	Type        string  `json:"type"`
	Severity    float64 `json:"severity"`
	Description string  `json:"description"`
	Mitigation  string  `json:"mitigation"`
}

// RecommendedApproach suggests the best transformation strategy
type RecommendedApproach struct {
	Strategy    StrategyType `json:"strategy"`
	Confidence  float64      `json:"confidence"`
	Reasoning   string       `json:"reasoning"`
	Alternatives []StrategyType `json:"alternatives"`
}

// DefaultHybridPipeline implements the hybrid transformation pipeline
type DefaultHybridPipeline struct {
	recipeExecutor      *RecipeExecutor
	llmGenerator        LLMRecipeGenerator
	multiLangEngine     MultiLanguageEngine
	strategySelector    StrategySelector
	complexityAnalyzer  ComplexityAnalyzer
	confidenceThresholds ConfidenceThresholds
	mu                  sync.RWMutex
}

// NewDefaultHybridPipeline creates a new hybrid pipeline
func NewDefaultHybridPipeline(
	recipeExecutor *RecipeExecutor,
	llmGenerator LLMRecipeGenerator,
	multiLangEngine MultiLanguageEngine,
) *DefaultHybridPipeline {
	return &DefaultHybridPipeline{
		recipeExecutor: recipeExecutor,
		llmGenerator:     llmGenerator,
		multiLangEngine:  multiLangEngine,
		strategySelector: NewDefaultStrategySelector(),
		complexityAnalyzer: NewDefaultComplexityAnalyzer(),
		confidenceThresholds: ConfidenceThresholds{
			MinOpenRewrite: 0.6,
			MinLLM:         0.7,
			MinHybrid:      0.8,
			RequiredBuild:  0.9,
		},
	}
}

// ExecuteHybridTransformation executes a transformation using the optimal hybrid strategy
func (p *DefaultHybridPipeline) ExecuteHybridTransformation(ctx context.Context, request HybridRequest) (*HybridResult, error) {
	startTime := time.Now()

	// Analyze complexity to select optimal strategy
	complexity, err := p.complexityAnalyzer.AnalyzeComplexity(ctx, request.Repository)
	if err != nil {
		return nil, fmt.Errorf("complexity analysis failed: %w", err)
	}

	strategy, err := p.SelectOptimalStrategy(ctx, *complexity)
	if err != nil {
		return nil, fmt.Errorf("strategy selection failed: %w", err)
	}

	// Execute based on selected strategy
	var result *HybridResult
	switch strategy.Primary {
	case StrategyOpenRewriteOnly:
		result, err = p.executeOpenRewriteOnly(ctx, request, *strategy)
	case StrategyLLMOnly:
		result, err = p.executeLLMOnly(ctx, request, *strategy)
	case StrategyHybridSequential:
		result, err = p.executeHybridSequential(ctx, request, *strategy)
	case StrategyHybridParallel:
		result, err = p.executeHybridParallel(ctx, request, *strategy)
	case StrategyTreeSitter:
		result, err = p.executeTreeSitterOnly(ctx, request, *strategy)
	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategy.Primary)
	}

	if err != nil {
		// Try fallback strategies
		for _, fallback := range strategy.Fallbacks {
			fallbackStrategy := &TransformationStrategy{Primary: fallback}
			if fallbackResult, fallbackErr := p.executeFallbackStrategy(ctx, request, *fallbackStrategy); fallbackErr == nil {
				result = fallbackResult
				break
			}
		}
		
		if result == nil {
			return nil, fmt.Errorf("all strategies failed: %w", err)
		}
	}

	result.Strategy = *strategy
	result.TotalTime = time.Since(startTime)
	
	// Calculate overall confidence score
	result.ConfidenceScore = p.calculateOverallConfidence(result)

	return result, nil
}

// SelectOptimalStrategy selects the best transformation strategy based on complexity analysis
func (p *DefaultHybridPipeline) SelectOptimalStrategy(ctx context.Context, analysis ComplexityAnalysis) (*TransformationStrategy, error) {
	// Use strategy selector to determine optimal approach
	request := StrategyRequest{
		TransformationType: TransformationType(analysis.RecommendedApproach.Strategy),
		ResourceConstraints: ResourceConstraints{
			MaxMemory: 4 * 1024 * 1024 * 1024, // 4GB default
			MaxCPU:    4,                       // 4 cores default
		},
		TimeConstraints: TimeConstraints{
			MaxDuration: 30 * time.Minute,
		},
		QualityRequirements: QualityRequirements{
			MinConfidence: p.confidenceThresholds.MinHybrid,
		},
	}

	selectedStrategy, err := p.strategySelector.SelectStrategy(ctx, request)
	if err != nil {
		return nil, err
	}
	return &selectedStrategy.Primary, nil
}

// EnhanceWithLLM enhances transformation results using LLM capabilities
func (p *DefaultHybridPipeline) EnhanceWithLLM(ctx context.Context, baseResult TransformationResult) (*EnhancedResult, error) {
	startTime := time.Now()

	// Build enhancement request from base result
	enhancementRequest := p.buildEnhancementRequest(baseResult)

	// Generate enhanced suggestions using LLM
	generatedRecipe, err := p.llmGenerator.GenerateRecipe(ctx, enhancementRequest)
	if err != nil {
		return nil, fmt.Errorf("LLM enhancement failed: %w", err)
	}

	// Convert LLM suggestions to code changes
	enhancedChanges := p.convertLLMSuggestionsToChanges(generatedRecipe)

	// Calculate confidence improvement
	confidenceImprovement := generatedRecipe.Confidence - baseResult.ValidationScore

	enhancedResult := &EnhancedResult{
		OriginalResult:        baseResult,
		EnhancedChanges:       enhancedChanges,
		LLMSuggestions:        p.convertToLLMSuggestions(generatedRecipe),
		ConfidenceImprovement: confidenceImprovement,
		EnhancementTime:       time.Since(startTime),
		EnhancementMetadata: map[string]interface{}{
			"llm_model":           generatedRecipe.LLMMetadata.Model,
			"prompt_tokens":       generatedRecipe.LLMMetadata.PromptTokens,
			"response_tokens":     generatedRecipe.LLMMetadata.ResponseTokens,
			"enhancement_reason":  generatedRecipe.Explanation,
		},
	}

	return enhancedResult, nil
}

// Strategy execution methods

func (p *DefaultHybridPipeline) executeOpenRewriteOnly(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// Execute using OpenRewrite engine only
	// ExecuteRecipeObject takes a recipe and path to repository
	// For now, use the repository URL as the path (in real implementation this would be a local checkout)
	// For now, use the repository URL as path - in real implementation this would be a cloned local path
	result, err := p.recipeExecutor.ExecuteRecipeObject(ctx, request.PrimaryRecipe, request.Repository.URL)
	
	if err != nil {
		return nil, fmt.Errorf("OpenRewrite execution failed: %w", err)
	}

	return &HybridResult{
		TransformationResult: *result,
		PrimaryResult:        result,
		EnhancedResult:       nil,
	}, nil
}

func (p *DefaultHybridPipeline) executeLLMOnly(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// Generate recipe using LLM and execute
	llmRequest := p.buildLLMRequestFromHybridRequest(request)
	
	generatedRecipe, err := p.llmGenerator.GenerateRecipe(ctx, llmRequest)
	if err != nil {
		return nil, fmt.Errorf("LLM recipe generation failed: %w", err)
	}

	// Validate generated recipe
	validation, err := p.llmGenerator.ValidateGenerated(ctx, *generatedRecipe)
	if err != nil {
		return nil, fmt.Errorf("LLM recipe validation failed: %w", err)
	}

	if !validation.Valid {
		return nil, fmt.Errorf("generated recipe failed validation: warnings=%v, critical_issues=%v", validation.Warnings, validation.CriticalIssues)
	}

	// Execute generated recipe (simulate for now)
	result := &TransformationResult{
		Success:           true,
		ExecutionTime:     time.Second * 30, // Simulated
		ChangesApplied:    1,
		FilesModified:     []string{"example.java"},
		ValidationScore:   generatedRecipe.Confidence,
		RecipeID:          generatedRecipe.Recipe.ID,
		Metadata: map[string]interface{}{
			"llm_generated": true,
			"confidence":    generatedRecipe.Confidence,
			"explanation":   generatedRecipe.Explanation,
		},
	}

	return &HybridResult{
		TransformationResult: *result,
		PrimaryResult:        result,
		EnhancedResult:       nil,
	}, nil
}

func (p *DefaultHybridPipeline) executeHybridSequential(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// First execute OpenRewrite
	primaryResult, err := p.executeOpenRewriteOnly(ctx, request, strategy)
	if err != nil {
		return nil, fmt.Errorf("primary OpenRewrite execution failed: %w", err)
	}

	// Then enhance with LLM
	enhancedResult, err := p.EnhanceWithLLM(ctx, primaryResult.TransformationResult)
	if err != nil {
		return nil, fmt.Errorf("LLM enhancement failed: %w", err)
	}

	// Combine results
	combinedResult := p.combineResults(primaryResult.TransformationResult, *enhancedResult)

	return &HybridResult{
		TransformationResult: *combinedResult,
		PrimaryResult:        &primaryResult.TransformationResult,
		EnhancedResult:       enhancedResult,
	}, nil
}

func (p *DefaultHybridPipeline) executeHybridParallel(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// Execute OpenRewrite and LLM in parallel
	type parallelResult struct {
		openRewriteResult *HybridResult
		llmResult         *HybridResult
		err               error
	}

	results := make(chan parallelResult, 2)
	
	// OpenRewrite execution
	go func() {
		result, err := p.executeOpenRewriteOnly(ctx, request, strategy)
		results <- parallelResult{openRewriteResult: result, err: err}
	}()

	// LLM execution
	go func() {
		result, err := p.executeLLMOnly(ctx, request, strategy)
		results <- parallelResult{llmResult: result, err: err}
	}()

	// Collect results
	var openRewriteResult, llmResult *HybridResult
	var errs []error

	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		
		if result.openRewriteResult != nil {
			openRewriteResult = result.openRewriteResult
		}
		if result.llmResult != nil {
			llmResult = result.llmResult
		}
	}

	// Handle errors
	if len(errs) == 2 {
		return nil, fmt.Errorf("both parallel executions failed: %v", errs)
	}

	// Use the better result or combine them
	if openRewriteResult != nil && llmResult != nil {
		// Both succeeded, combine results
		combinedResult := p.combineParallelResults(openRewriteResult.TransformationResult, llmResult.TransformationResult)
		return &HybridResult{
			TransformationResult: *combinedResult,
			PrimaryResult:        &openRewriteResult.TransformationResult,
		}, nil
	}

	// One succeeded, use that result
	if openRewriteResult != nil {
		return openRewriteResult, nil
	}
	if llmResult != nil {
		return llmResult, nil
	}

	return nil, fmt.Errorf("unexpected parallel execution state")
}

func (p *DefaultHybridPipeline) executeTreeSitterOnly(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// Parse code using tree-sitter
	ast, err := p.multiLangEngine.ParseAST(ctx, "// placeholder code", request.Repository.Language)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parsing failed: %w", err)
	}

	// Generate transformation using multi-language engine
	transformation, err := p.multiLangEngine.GenerateTransformation(ctx, ast, request.PrimaryRecipe)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter transformation failed: %w", err)
	}

	// Convert transformation to result
	result := &TransformationResult{
		Success:         true,
		ExecutionTime:   time.Second * 15, // Simulated
		ChangesApplied:  len(transformation.Changes),
		FilesModified:   []string{"example." + p.getFileExtension(request.Repository.Language)},
		ValidationScore: 0.85, // Tree-sitter typically has good accuracy
		RecipeID:        request.PrimaryRecipe.ID,
		Metadata: map[string]interface{}{
			"tree_sitter_ast": true,
			"language":        ast.Language,
			"parser":          ast.Parser,
			"symbols_found":   len(ast.Symbols),
			"imports_found":   len(ast.Imports),
		},
	}

	return &HybridResult{
		TransformationResult: *result,
		PrimaryResult:        result,
		EnhancedResult:       nil,
	}, nil
}

func (p *DefaultHybridPipeline) executeFallbackStrategy(ctx context.Context, request HybridRequest, strategy TransformationStrategy) (*HybridResult, error) {
	// Execute fallback strategy
	switch strategy.Primary {
	case StrategyOpenRewriteOnly:
		return p.executeOpenRewriteOnly(ctx, request, strategy)
	case StrategyLLMOnly:
		return p.executeLLMOnly(ctx, request, strategy)
	case StrategyTreeSitter:
		return p.executeTreeSitterOnly(ctx, request, strategy)
	default:
		return nil, fmt.Errorf("unsupported fallback strategy: %s", strategy.Primary)
	}
}

// Helper methods

func (p *DefaultHybridPipeline) buildEnhancementRequest(baseResult TransformationResult) RecipeGenerationRequest {
	return RecipeGenerationRequest{
		ErrorContext: ErrorContext{
			ErrorType:    "enhancement_needed",
			ErrorMessage: "Base transformation needs improvement",
		},
		CodebaseContext: CodebaseContext{
			Language: "java", // Default, should be extracted from baseResult
		},
		Constraints: []RecipeConstraint{
			{
				Type:        "improve_confidence",
				Description: "Enhance the existing transformation result",
				Required:    true,
			},
		},
	}
}

func (p *DefaultHybridPipeline) convertLLMSuggestionsToChanges(generatedRecipe *GeneratedRecipe) []CodeChange {
	// Convert LLM-generated recipe to code changes
	// This is a simplified implementation
	return []CodeChange{
		{
			Type:        "improve",
			OldText:     "// placeholder",
			NewText:     "// LLM enhanced code",
			Explanation: generatedRecipe.Explanation,
		},
	}
}

func (p *DefaultHybridPipeline) convertToLLMSuggestions(generatedRecipe *GeneratedRecipe) []LLMSuggestion {
	return []LLMSuggestion{
		{
			Type:        "enhancement",
			Confidence:  generatedRecipe.Confidence,
			Description: generatedRecipe.Explanation,
			CodeChange: CodeChange{
				Type:        "improve",
				Explanation: generatedRecipe.Explanation,
			},
			Reasoning: "LLM-generated improvement based on analysis",
		},
	}
}

func (p *DefaultHybridPipeline) buildLLMRequestFromHybridRequest(request HybridRequest) RecipeGenerationRequest {
	return RecipeGenerationRequest{
		Language: request.Repository.Language,
		CodebaseContext: CodebaseContext{
			Language:    request.Repository.Language,
			Framework:   request.Repository.Metadata["framework"],
			BuildTool:   request.Repository.BuildTool,
		},
		Constraints: []RecipeConstraint{
			{
				Type:        "hybrid_request",
				Description: "Generate recipe for hybrid transformation",
				Required:    true,
			},
		},
	}
}

func (p *DefaultHybridPipeline) combineResults(primary TransformationResult, enhanced EnhancedResult) *TransformationResult {
	combined := primary
	
	// Merge changes
	combined.ChangesApplied += len(enhanced.EnhancedChanges)
	
	// Improve confidence
	if enhanced.ConfidenceImprovement > 0 {
		combined.ValidationScore += enhanced.ConfidenceImprovement
		if combined.ValidationScore > 1.0 {
			combined.ValidationScore = 1.0
		}
	}

	// Add enhancement metadata
	if combined.Metadata == nil {
		combined.Metadata = make(map[string]interface{})
	}
	combined.Metadata["enhanced"] = true
	combined.Metadata["enhancement_time"] = enhanced.EnhancementTime
	combined.Metadata["llm_suggestions"] = len(enhanced.LLMSuggestions)

	return &combined
}

func (p *DefaultHybridPipeline) combineParallelResults(openRewriteResult, llmResult TransformationResult) *TransformationResult {
	// Choose the result with higher validation score
	if openRewriteResult.ValidationScore >= llmResult.ValidationScore {
		return &openRewriteResult
	}
	return &llmResult
}

func (p *DefaultHybridPipeline) calculateOverallConfidence(result *HybridResult) float64 {
	confidence := result.ValidationScore

	// Boost confidence if enhanced
	if result.EnhancedResult != nil {
		confidence += result.EnhancedResult.ConfidenceImprovement * 0.5
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func (p *DefaultHybridPipeline) getFileExtension(language string) string {
	extensions := map[string]string{
		"java":       "java",
		"javascript": "js",
		"typescript": "ts",
		"python":     "py",
		"go":         "go",
		"rust":       "rs",
	}
	
	if ext, exists := extensions[language]; exists {
		return ext
	}
	return "txt"
}