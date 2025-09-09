package arf

import (
	"fmt"
	"math"
	"strings"
)

// calculateLanguageComplexity determines complexity based on programming language
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

// calculateFrameworkComplexity determines complexity based on framework usage
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

// calculateSizeComplexity determines complexity based on repository size
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

// calculateDependencyComplexity determines complexity based on project dependencies
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

// getDependencyComplexity returns complexity score for a specific dependency
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

// calculateBuildComplexity determines complexity based on build tool
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

// calculateTestComplexity determines complexity based on test coverage
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

// predictChallenges identifies potential challenges based on complexity factors
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

// generateRecommendedApproach determines the best transformation strategy
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
