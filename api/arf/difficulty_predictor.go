package arf

import (
	"time"
)

// getBaseDifficulty returns base difficulty score for transformation type
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

// adjustDifficultyForRepository adjusts base difficulty based on repository characteristics
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

// calculateSuccessProbability calculates probability of transformation success
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

// identifyKeyChallenges identifies key challenges for transformation
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

// estimateEffort estimates time required for transformation
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

// recommendTransformationApproach recommends sequence of transformations
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

// calculateConfidenceLevel calculates confidence in analysis results
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

// containsTransformationType checks if transformation type is in slice
func (c *DefaultComplexityAnalyzer) containsTransformationType(slice []TransformationType, item TransformationType) bool {
	for _, t := range slice {
		if t == item {
			return true
		}
	}
	return false
}
