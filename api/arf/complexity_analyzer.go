package arf

import (
	"context"
	"fmt"
)

// DefaultComplexityAnalyzer implements complexity analysis
type DefaultComplexityAnalyzer struct {
	multiLangEngine MultiLanguageEngine
}

// NewDefaultComplexityAnalyzer creates a new complexity analyzer
func NewDefaultComplexityAnalyzer() *DefaultComplexityAnalyzer {
	return &DefaultComplexityAnalyzer{}
}

// SetMultiLanguageEngine sets the multi-language engine for AST analysis
func (c *DefaultComplexityAnalyzer) SetMultiLanguageEngine(engine MultiLanguageEngine) {
	c.multiLangEngine = engine
}

// AnalyzeComplexity performs comprehensive complexity analysis of a repository
func (c *DefaultComplexityAnalyzer) AnalyzeComplexity(ctx context.Context, repository Repository) (*ComplexityAnalysis, error) {
	// Initialize analysis
	analysis := &ComplexityAnalysis{
		FactorBreakdown:     make(map[string]float64),
		PredictedChallenges: []PredictedChallenge{},
	}

	// Language complexity factor
	languageComplexity := c.calculateLanguageComplexity(repository.Language)
	analysis.FactorBreakdown["language"] = languageComplexity

	// Framework complexity factor
	framework := repository.Metadata["framework"]
	if framework == "" {
		framework = repository.BuildTool
	}
	frameworkComplexity := c.calculateFrameworkComplexity(framework, repository.Language)
	analysis.FactorBreakdown["framework"] = frameworkComplexity

	// Repository size factor
	sizeComplexity := c.calculateSizeComplexity(repository)
	analysis.FactorBreakdown["size"] = sizeComplexity

	// Dependency complexity factor
	dependencyComplexity := c.calculateDependencyComplexity(repository.Dependencies)
	analysis.FactorBreakdown["dependencies"] = dependencyComplexity

	// Build tool complexity factor
	buildComplexity := c.calculateBuildComplexity(repository.BuildTool, repository.Language)
	analysis.FactorBreakdown["build_tool"] = buildComplexity

	// Test coverage factor (higher coverage = lower complexity for transformation)
	testCoverage := 0.7 // Default test coverage assumption
	if tcStr, exists := repository.Metadata["test_coverage"]; exists {
		if tc, err := fmt.Sscanf(tcStr, "%f", &testCoverage); err == nil && tc == 1 {
			// Successfully parsed test coverage
		}
	}
	testComplexity := c.calculateTestComplexity(testCoverage)
	analysis.FactorBreakdown["test_coverage"] = testComplexity

	// Calculate overall complexity (weighted average)
	weights := map[string]float64{
		"language":      0.25,
		"framework":     0.20,
		"size":          0.15,
		"dependencies":  0.20,
		"build_tool":    0.10,
		"test_coverage": 0.10,
	}

	overallComplexity := 0.0
	for factor, value := range analysis.FactorBreakdown {
		if weight, exists := weights[factor]; exists {
			overallComplexity += value * weight
		}
	}

	analysis.OverallComplexity = overallComplexity

	// Predict challenges based on complexity factors
	analysis.PredictedChallenges = c.predictChallenges(analysis.FactorBreakdown, repository)

	// Generate recommended approach
	analysis.RecommendedApproach = c.generateRecommendedApproach(overallComplexity, repository)

	return analysis, nil
}

// AnalyzeCodeComplexity analyzes complexity of specific code
func (c *DefaultComplexityAnalyzer) AnalyzeCodeComplexity(ctx context.Context, code string, language string) (*CodeComplexityMetrics, error) {
	metrics := &CodeComplexityMetrics{}

	// Basic metrics
	metrics.LinesOfCode = c.countLinesOfCode(code)
	metrics.FunctionCount = c.countFunctions(code, language)
	metrics.ClassCount = c.countClasses(code, language)
	metrics.NestingDepth = c.calculateNestingDepth(code, language)

	// If multi-language engine is available, use AST for more accurate analysis
	if c.multiLangEngine != nil {
		if supported, _ := c.multiLangEngine.ValidateLanguageSupport(language); supported {
			ast, err := c.multiLangEngine.ParseAST(ctx, code, language)
			if err == nil {
				metrics = c.extractMetricsFromAST(ast, metrics)
			}
		}
	}

	// Calculate derived metrics
	metrics.CyclomaticComplexity = c.calculateCyclomaticComplexity(code, language)
	metrics.CognitiveComplexity = c.calculateCognitiveComplexity(code, language)
	metrics.MaintainabilityIndex = c.calculateMaintainabilityIndex(metrics)

	return metrics, nil
}

// PredictDifficulty predicts the difficulty of a specific transformation
func (c *DefaultComplexityAnalyzer) PredictDifficulty(ctx context.Context, transformation TransformationType, repository Repository) (*DifficultyPrediction, error) {
	// Base difficulty by transformation type
	baseDifficulty := c.getBaseDifficulty(transformation)

	// Repository complexity analysis
	repoAnalysis, err := c.AnalyzeComplexity(ctx, repository)
	if err != nil {
		return nil, fmt.Errorf("repository analysis failed: %w", err)
	}

	// Adjust difficulty based on repository characteristics
	adjustedDifficulty := c.adjustDifficultyForRepository(baseDifficulty, repoAnalysis, repository)

	// Predict success probability
	successProbability := c.calculateSuccessProbability(adjustedDifficulty, transformation, repository)

	// Identify key challenges
	keyChallenges := c.identifyKeyChallenges(transformation, repoAnalysis, repository)

	// Estimate effort
	repoSize := 1000 // Default repository size in lines of code
	if sizeStr, exists := repository.Metadata["size"]; exists {
		if size, err := fmt.Sscanf(sizeStr, "%d", &repoSize); err == nil && size == 1 {
			// Successfully parsed repository size
		}
	}
	estimatedEffort := c.estimateEffort(adjustedDifficulty, int64(repoSize))

	// Recommend approach
	recommendedApproach := c.recommendTransformationApproach(transformation, adjustedDifficulty, repository)

	return &DifficultyPrediction{
		OverallDifficulty:   adjustedDifficulty,
		ConfidenceLevel:     c.calculateConfidenceLevel(repoAnalysis),
		KeyChallenges:       keyChallenges,
		EstimatedEffort:     estimatedEffort,
		SuccessProbability:  successProbability,
		RecommendedApproach: recommendedApproach,
	}, nil
}
