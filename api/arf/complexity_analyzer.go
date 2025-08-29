package arf

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// ComplexityAnalyzer defines the interface for analyzing transformation complexity
type ComplexityAnalyzer interface {
	AnalyzeComplexity(ctx context.Context, repository Repository) (*ComplexityAnalysis, error)
	AnalyzeCodeComplexity(ctx context.Context, code string, language string) (*CodeComplexityMetrics, error)
	PredictDifficulty(ctx context.Context, transformation TransformationType, repository Repository) (*DifficultyPrediction, error)
}

// CodeComplexityMetrics contains metrics about code complexity
type CodeComplexityMetrics struct {
	CyclomaticComplexity int     `json:"cyclomatic_complexity"`
	LinesOfCode          int     `json:"lines_of_code"`
	FunctionCount        int     `json:"function_count"`
	ClassCount           int     `json:"class_count"`
	NestingDepth         int     `json:"nesting_depth"`
	CognitiveComplexity  int     `json:"cognitive_complexity"`
	MaintainabilityIndex float64 `json:"maintainability_index"`
}

// DifficultyPrediction predicts transformation difficulty
type DifficultyPrediction struct {
	OverallDifficulty   float64              `json:"overall_difficulty"`
	ConfidenceLevel     float64              `json:"confidence_level"`
	KeyChallenges       []string             `json:"key_challenges"`
	EstimatedEffort     time.Duration        `json:"estimated_effort"`
	SuccessProbability  float64              `json:"success_probability"`
	RecommendedApproach []TransformationType `json:"recommended_approach"`
}

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

// Helper methods for complexity calculation

func (c *DefaultComplexityAnalyzer) calculateLanguageComplexity(language string) float64 {
	// Language complexity based on transformation difficulty
	languageScores := map[string]float64{
		"java":       0.4, // Well-supported by OpenRewrite
		"javascript": 0.5, // Moderate complexity
		"typescript": 0.5, // Similar to JavaScript
		"python":     0.6, // Dynamic typing adds complexity
		"go":         0.3, // Simple language structure
		"rust":       0.7, // Complex type system
		"c":          0.8, // Low-level, manual memory management
		"cpp":        0.9, // Very complex
		"scala":      0.8, // Complex type system and syntax
		"kotlin":     0.5, // Similar to Java but newer
	}

	if score, exists := languageScores[strings.ToLower(language)]; exists {
		return score
	}
	return 0.7 // Default for unknown languages
}

func (c *DefaultComplexityAnalyzer) calculateFrameworkComplexity(framework, language string) float64 {
	// Framework-specific complexity scores
	frameworkScores := map[string]map[string]float64{
		"java": {
			"spring":      0.3, // Well-supported
			"spring-boot": 0.2, // Very well-supported
			"junit":       0.1, // Testing framework, simple
			"maven":       0.2, // Build tool complexity
			"gradle":      0.4, // More complex build tool
		},
		"javascript": {
			"react":   0.4,
			"vue":     0.3,
			"angular": 0.6, // More complex
			"node":    0.3,
			"express": 0.2,
		},
		"python": {
			"django":  0.5,
			"flask":   0.3,
			"fastapi": 0.3,
			"pytest":  0.2,
		},
		"go": {
			"gin":     0.2,
			"echo":    0.2,
			"gorilla": 0.3,
		},
	}

	langFrameworks, exists := frameworkScores[strings.ToLower(language)]
	if !exists {
		return 0.5 // Default
	}

	if score, exists := langFrameworks[strings.ToLower(framework)]; exists {
		return score
	}
	return 0.4 // Default for unknown framework
}

func (c *DefaultComplexityAnalyzer) calculateSizeComplexity(repository Repository) float64 {
	// Size complexity based on lines of code and file count
	// Get repository size from metadata or use defaults
	repoSize := 1000 // Default size in lines of code
	fileCount := 50  // Default file count

	if sizeStr, exists := repository.Metadata["size"]; exists {
		if size, err := fmt.Sscanf(sizeStr, "%d", &repoSize); err == nil && size == 1 {
			// Successfully parsed repository size
		}
	}

	if fileCountStr, exists := repository.Metadata["file_count"]; exists {
		if count, err := fmt.Sscanf(fileCountStr, "%d", &fileCount); err == nil && count == 1 {
			// Successfully parsed file count
		}
	}

	sizeBytes := float64(repoSize)
	fileCountFloat := float64(fileCount)

	// Normalize size (assuming 1MB = moderate complexity)
	sizeComplexity := math.Min(1.0, sizeBytes/(1024*1024))

	// File count factor (more files = more complexity)
	fileComplexity := math.Min(1.0, fileCountFloat/100) // 100 files = high complexity

	// Combined score
	return (sizeComplexity + fileComplexity) / 2
}

func (c *DefaultComplexityAnalyzer) calculateDependencyComplexity(dependencies []string) float64 {
	if len(dependencies) == 0 {
		return 0.1 // Very low complexity
	}

	// Base complexity from dependency count
	depCount := float64(len(dependencies))
	countComplexity := math.Min(1.0, depCount/20) // 20 deps = high complexity

	// Analyze specific dependencies for known complexity
	depComplexity := 0.0
	for _, dep := range dependencies {
		depComplexity += c.getDependencyComplexity(dep)
	}
	depComplexity = depComplexity / depCount // Average

	return (countComplexity + depComplexity) / 2
}

func (c *DefaultComplexityAnalyzer) getDependencyComplexity(dependency string) float64 {
	// Known complex dependencies
	complexDeps := map[string]float64{
		"spring-security": 0.8,
		"hibernate":       0.7,
		"react":           0.4,
		"angular":         0.6,
		"tensorflow":      0.9,
		"pytorch":         0.9,
		"kubernetes":      0.9,
		"docker":          0.3,
	}

	depName := strings.ToLower(dependency)
	for pattern, complexity := range complexDeps {
		if strings.Contains(depName, pattern) {
			return complexity
		}
	}

	return 0.3 // Default for unknown dependencies
}

func (c *DefaultComplexityAnalyzer) calculateBuildComplexity(buildTool, language string) float64 {
	buildScores := map[string]map[string]float64{
		"java": {
			"maven":  0.2,
			"gradle": 0.4,
			"ant":    0.6, // Older, more complex
		},
		"javascript": {
			"npm":     0.2,
			"yarn":    0.2,
			"webpack": 0.5,
			"rollup":  0.4,
		},
		"python": {
			"pip":        0.2,
			"poetry":     0.3,
			"setuptools": 0.4,
		},
		"go": {
			"go-mod": 0.1, // Very simple
			"dep":    0.3, // Deprecated but more complex
		},
	}

	langBuilds, exists := buildScores[strings.ToLower(language)]
	if !exists {
		return 0.3 // Default
	}

	if score, exists := langBuilds[strings.ToLower(buildTool)]; exists {
		return score
	}
	return 0.3 // Default
}

func (c *DefaultComplexityAnalyzer) calculateTestComplexity(testCoverage float64) float64 {
	// Lower test coverage = higher transformation complexity
	// Invert the coverage (1.0 - coverage) to get complexity
	if testCoverage < 0 {
		testCoverage = 0
	}
	if testCoverage > 1 {
		testCoverage = 1
	}

	// Good test coverage reduces transformation risk/complexity
	return 1.0 - testCoverage
}

func (c *DefaultComplexityAnalyzer) predictChallenges(factors map[string]float64, repository Repository) []PredictedChallenge {
	var challenges []PredictedChallenge

	// Language-specific challenges
	if factors["language"] > 0.6 {
		challenges = append(challenges, PredictedChallenge{
			Type:        "language_complexity",
			Severity:    factors["language"],
			Description: fmt.Sprintf("High complexity language: %s", repository.Language),
			Mitigation:  "Consider hybrid approach with LLM assistance",
		})
	}

	// Framework challenges
	if factors["framework"] > 0.5 {
		framework := repository.Metadata["framework"]
		if framework == "" {
			framework = repository.BuildTool
		}
		challenges = append(challenges, PredictedChallenge{
			Type:        "framework_complexity",
			Severity:    factors["framework"],
			Description: fmt.Sprintf("Complex framework: %s", framework),
			Mitigation:  "Use framework-specific recipes and validation",
		})
	}

	// Size challenges
	if factors["size"] > 0.7 {
		challenges = append(challenges, PredictedChallenge{
			Type:        "large_codebase",
			Severity:    factors["size"],
			Description: "Large codebase may require staged transformation",
			Mitigation:  "Break into smaller chunks, use parallel processing",
		})
	}

	// Dependency challenges
	if factors["dependencies"] > 0.6 {
		challenges = append(challenges, PredictedChallenge{
			Type:        "dependency_complexity",
			Severity:    factors["dependencies"],
			Description: "Complex dependency graph",
			Mitigation:  "Analyze dependency compatibility before transformation",
		})
	}

	// Test coverage challenges
	if factors["test_coverage"] > 0.5 {
		challenges = append(challenges, PredictedChallenge{
			Type:        "low_test_coverage",
			Severity:    factors["test_coverage"],
			Description: "Low test coverage increases transformation risk",
			Mitigation:  "Add tests before transformation or use conservative approach",
		})
	}

	return challenges
}

func (c *DefaultComplexityAnalyzer) generateRecommendedApproach(complexity float64, repository Repository) RecommendedApproach {
	var strategy StrategyType
	var confidence float64
	var reasoning string
	var alternatives []StrategyType

	if complexity < 0.3 {
		// Low complexity - OpenRewrite should handle well
		strategy = StrategyOpenRewriteOnly
		confidence = 0.9
		reasoning = "Low complexity allows for deterministic transformation"
		alternatives = []StrategyType{StrategyTreeSitter}
	} else if complexity < 0.6 {
		// Medium complexity - hybrid approach recommended
		strategy = StrategyHybridSequential
		confidence = 0.8
		reasoning = "Medium complexity benefits from hybrid approach"
		alternatives = []StrategyType{StrategyOpenRewriteOnly, StrategyLLMOnly}
	} else {
		// High complexity - need advanced techniques
		strategy = StrategyLLMOnly
		confidence = 0.7
		reasoning = "High complexity requires AI-assisted transformation"
		alternatives = []StrategyType{StrategyHybridSequential, StrategyTreeSitter}
	}

	return RecommendedApproach{
		Strategy:     strategy,
		Confidence:   confidence,
		Reasoning:    reasoning,
		Alternatives: alternatives,
	}
}

// Code analysis helper methods

func (c *DefaultComplexityAnalyzer) countLinesOfCode(code string) int {
	lines := strings.Split(code, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	return nonEmptyLines
}

func (c *DefaultComplexityAnalyzer) countFunctions(code string, language string) int {
	// Simple pattern-based counting - could be enhanced with AST
	patterns := map[string][]string{
		"java":       {"public ", "private ", "protected ", "static "},
		"javascript": {"function ", "const ", "let ", "=> "},
		"python":     {"def "},
		"go":         {"func "},
		"rust":       {"fn "},
	}

	count := 0
	if langPatterns, exists := patterns[language]; exists {
		for _, pattern := range langPatterns {
			count += strings.Count(strings.ToLower(code), pattern)
		}
	}

	return count / 2 // Rough estimate, divide by 2 to account for overcount
}

func (c *DefaultComplexityAnalyzer) countClasses(code string, language string) int {
	// Simple pattern-based counting
	patterns := map[string][]string{
		"java":       {"class ", "interface ", "enum "},
		"javascript": {"class "},
		"typescript": {"class ", "interface "},
		"python":     {"class "},
		"go":         {"type ", "struct "},
		"rust":       {"struct ", "enum ", "trait "},
	}

	count := 0
	if langPatterns, exists := patterns[language]; exists {
		for _, pattern := range langPatterns {
			count += strings.Count(strings.ToLower(code), pattern)
		}
	}

	return count
}

func (c *DefaultComplexityAnalyzer) calculateNestingDepth(code string, language string) int {
	// Simple brace counting for nesting depth
	maxDepth := 0
	currentDepth := 0

	for _, char := range code {
		if char == '{' {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		} else if char == '}' {
			currentDepth--
		}
	}

	return maxDepth
}

func (c *DefaultComplexityAnalyzer) calculateCyclomaticComplexity(code string, language string) int {
	// Simplified cyclomatic complexity
	// Count decision points: if, while, for, switch, catch
	decisionKeywords := []string{"if", "while", "for", "switch", "catch", "elif", "else if"}

	complexity := 1 // Base complexity
	codeLowar := strings.ToLower(code)

	for _, keyword := range decisionKeywords {
		complexity += strings.Count(codeLowar, keyword)
	}

	return complexity
}

func (c *DefaultComplexityAnalyzer) calculateCognitiveComplexity(code string, language string) int {
	// Simplified cognitive complexity
	// Similar to cyclomatic but with nesting penalties
	complexity := c.calculateCyclomaticComplexity(code, language)
	nestingDepth := c.calculateNestingDepth(code, language)

	// Add penalty for deep nesting
	return complexity + nestingDepth
}

func (c *DefaultComplexityAnalyzer) calculateMaintainabilityIndex(metrics *CodeComplexityMetrics) float64 {
	// Simplified maintainability index
	// Based on Halstead metrics approximation and cyclomatic complexity
	if metrics.LinesOfCode == 0 {
		return 100.0
	}

	// Approximate calculation
	complexity := float64(metrics.CyclomaticComplexity)
	loc := float64(metrics.LinesOfCode)

	// Higher complexity and more lines = lower maintainability
	maintainability := 100.0 - (complexity * 5) - (loc / 10)

	if maintainability < 0 {
		maintainability = 0
	}
	if maintainability > 100 {
		maintainability = 100
	}

	return maintainability
}

func (c *DefaultComplexityAnalyzer) extractMetricsFromAST(ast *UniversalAST, metrics *CodeComplexityMetrics) *CodeComplexityMetrics {
	// Enhance metrics using AST data
	if ast != nil {
		// More accurate function count from symbols
		functionSymbols := 0
		classSymbols := 0

		for _, symbol := range ast.Symbols {
			switch symbol.Type {
			case "function", "method":
				functionSymbols++
			case "class", "struct":
				classSymbols++
			}
		}

		// Use AST data if it seems more accurate
		if functionSymbols > 0 {
			metrics.FunctionCount = functionSymbols
		}
		if classSymbols > 0 {
			metrics.ClassCount = classSymbols
		}
	}

	return metrics
}

// Difficulty prediction helper methods

func (c *DefaultComplexityAnalyzer) getBaseDifficulty(transformation TransformationType) float64 {
	difficulties := map[TransformationType]float64{
		TransformationTypeCleanup:   0.2, // Generally easy
		TransformationTypeModernize: 0.5, // Medium complexity
		TransformationTypeMigration: 0.8, // Generally difficult
		TransformationTypeSecurity:  0.6, // Medium-high complexity
		TransformationTypeRefactor:  0.7, // High complexity
		TransformationTypeOptimize:  0.4, // Medium complexity
		TransformationTypeWASM:      0.9, // Very complex
	}

	if difficulty, exists := difficulties[transformation]; exists {
		return difficulty
	}
	return 0.5 // Default medium difficulty
}

func (c *DefaultComplexityAnalyzer) adjustDifficultyForRepository(baseDifficulty float64, analysis *ComplexityAnalysis, repository Repository) float64 {
	// Adjust base difficulty based on repository characteristics
	repoFactor := analysis.OverallComplexity

	// Weighted combination
	adjusted := 0.7*baseDifficulty + 0.3*repoFactor

	// Ensure within bounds
	if adjusted > 1.0 {
		adjusted = 1.0
	}
	if adjusted < 0.0 {
		adjusted = 0.0
	}

	return adjusted
}

func (c *DefaultComplexityAnalyzer) calculateSuccessProbability(difficulty float64, transformation TransformationType, repository Repository) float64 {
	// Base success probability (inverse of difficulty)
	baseSuccess := 1.0 - difficulty

	// Adjust based on transformation type confidence
	transformationBonus := 0.0
	switch transformation {
	case TransformationTypeCleanup:
		transformationBonus = 0.1 // Cleanup is usually safe
	case TransformationTypeSecurity:
		transformationBonus = -0.1 // Security changes can be risky
	}

	success := baseSuccess + transformationBonus

	// Ensure within bounds
	if success > 1.0 {
		success = 1.0
	}
	if success < 0.0 {
		success = 0.0
	}

	return success
}

func (c *DefaultComplexityAnalyzer) identifyKeyChallenges(transformation TransformationType, analysis *ComplexityAnalysis, repository Repository) []string {
	var challenges []string

	// Add transformation-specific challenges
	switch transformation {
	case TransformationTypeMigration:
		challenges = append(challenges, "API compatibility", "dependency updates", "breaking changes")
	case TransformationTypeSecurity:
		challenges = append(challenges, "security rule validation", "vulnerability assessment")
	case TransformationTypeWASM:
		challenges = append(challenges, "WASM compilation", "runtime compatibility", "memory management")
	}

	// Add repository-specific challenges from analysis
	for _, challenge := range analysis.PredictedChallenges {
		challenges = append(challenges, challenge.Description)
	}

	// Limit to top 5 challenges
	if len(challenges) > 5 {
		challenges = challenges[:5]
	}

	return challenges
}

func (c *DefaultComplexityAnalyzer) estimateEffort(difficulty float64, repositorySize int64) time.Duration {
	// Base effort estimation
	baseMinutes := 10 + int(difficulty*60) // 10-70 minutes base

	// Size factor
	sizeFactor := float64(repositorySize) / (1024 * 1024) // MB
	sizeMinutes := int(sizeFactor * 2)                    // 2 minutes per MB

	totalMinutes := baseMinutes + sizeMinutes

	// Cap at reasonable limits
	if totalMinutes > 300 { // 5 hours max
		totalMinutes = 300
	}
	if totalMinutes < 5 { // 5 minutes min
		totalMinutes = 5
	}

	return time.Duration(totalMinutes) * time.Minute
}

func (c *DefaultComplexityAnalyzer) recommendTransformationApproach(transformation TransformationType, difficulty float64, repository Repository) []TransformationType {
	var approaches []TransformationType

	// Primary approach based on difficulty
	if difficulty < 0.4 {
		approaches = append(approaches, TransformationTypeCleanup)
	} else if difficulty < 0.7 {
		approaches = append(approaches, TransformationTypeModernize, TransformationTypeRefactor)
	} else {
		approaches = append(approaches, TransformationTypeMigration, TransformationTypeSecurity)
	}

	// Add specific approach if not already included
	if !c.containsTransformationType(approaches, transformation) {
		approaches = append(approaches, transformation)
	}

	return approaches
}

func (c *DefaultComplexityAnalyzer) calculateConfidenceLevel(analysis *ComplexityAnalysis) float64 {
	// Confidence decreases with complexity and number of predicted challenges
	baseConfidence := 0.9

	complexityPenalty := analysis.OverallComplexity * 0.3
	challengesPenalty := float64(len(analysis.PredictedChallenges)) * 0.05

	confidence := baseConfidence - complexityPenalty - challengesPenalty

	if confidence < 0.1 {
		confidence = 0.1
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func (c *DefaultComplexityAnalyzer) containsTransformationType(slice []TransformationType, item TransformationType) bool {
	for _, t := range slice {
		if t == item {
			return true
		}
	}
	return false
}
