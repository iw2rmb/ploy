package arf

import (
	"context"
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
