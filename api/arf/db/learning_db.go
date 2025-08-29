package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LearningDB provides type-safe database operations for the learning system
type LearningDB struct {
	pool *pgxpool.Pool
}

// NewLearningDB creates a new learning database instance
func NewLearningDB(pool *pgxpool.Pool) *LearningDB {
	return &LearningDB{pool: pool}
}

// TransformationOutcome represents a transformation outcome record
type TransformationOutcome struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TransformationID  string          `json:"transformation_id" db:"transformation_id"`
	RecipeID          string          `json:"recipe_id" db:"recipe_id"`
	Success           bool            `json:"success" db:"success"`
	DurationSeconds   int32           `json:"duration_seconds" db:"duration_seconds"`
	Language          string          `json:"language" db:"language"`
	Framework         *string         `json:"framework,omitempty" db:"framework"`
	PatternSignature  string          `json:"pattern_signature" db:"pattern_signature"`
	CodebaseSize      *int32          `json:"codebase_size,omitempty" db:"codebase_size"`
	ComplexityScore   *float64        `json:"complexity_score,omitempty" db:"complexity_score"`
	Strategy          string          `json:"strategy" db:"strategy"`
	ErrorType         *string         `json:"error_type,omitempty" db:"error_type"`
	ErrorMessage      *string         `json:"error_message,omitempty" db:"error_message"`
	PerformanceImpact *float64        `json:"performance_impact,omitempty" db:"performance_impact"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	Metadata          json.RawMessage `json:"metadata" db:"metadata"`
}

// SuccessPattern represents a learned success pattern
type SuccessPattern struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	Signature       string          `json:"signature" db:"signature"`
	Language        string          `json:"language" db:"language"`
	SuccessRate     float64         `json:"success_rate" db:"success_rate"`
	OccurrenceCount int32           `json:"occurrence_count" db:"occurrence_count"`
	AvgDuration     float64         `json:"avg_duration" db:"avg_duration"`
	ConfidenceLevel float64         `json:"confidence_level" db:"confidence_level"`
	Factors         json.RawMessage `json:"factors" db:"factors"`
	Conditions      json.RawMessage `json:"conditions" db:"conditions"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
}

// FailurePattern represents a learned failure pattern
type FailurePattern struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Signature      string    `json:"signature" db:"signature"`
	Frequency      int32     `json:"frequency" db:"frequency"`
	FailureRate    float64   `json:"failure_rate" db:"failure_rate"`
	CommonErrors   []string  `json:"common_errors" db:"common_errors"`
	ContextFactors []string  `json:"context_factors" db:"context_factors"`
	Mitigations    []string  `json:"mitigations" db:"mitigations"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// StrategyWeight represents strategy performance weights
type StrategyWeight struct {
	ID               uuid.UUID `json:"id" db:"id"`
	StrategyName     string    `json:"strategy_name" db:"strategy_name"`
	Language         string    `json:"language" db:"language"`
	PatternType      string    `json:"pattern_type" db:"pattern_type"`
	Weight           float64   `json:"weight" db:"weight"`
	PerformanceScore float64   `json:"performance_score" db:"performance_score"`
	SampleSize       int32     `json:"sample_size" db:"sample_size"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// CreateTransformationOutcome inserts a new transformation outcome
func (db *LearningDB) CreateTransformationOutcome(ctx context.Context, outcome *TransformationOutcome) (*TransformationOutcome, error) {
	query := `
		INSERT INTO transformation_outcomes (
			transformation_id, recipe_id, success, duration_seconds,
			language, framework, pattern_signature, codebase_size,
			complexity_score, strategy, error_type, error_message,
			performance_impact, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING id, created_at`

	err := db.pool.QueryRow(ctx, query,
		outcome.TransformationID, outcome.RecipeID, outcome.Success, outcome.DurationSeconds,
		outcome.Language, outcome.Framework, outcome.PatternSignature, outcome.CodebaseSize,
		outcome.ComplexityScore, outcome.Strategy, outcome.ErrorType, outcome.ErrorMessage,
		outcome.PerformanceImpact, outcome.Metadata,
	).Scan(&outcome.ID, &outcome.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create transformation outcome: %w", err)
	}

	return outcome, nil
}

// GetSuccessPatternsByLanguage retrieves success patterns for a language
func (db *LearningDB) GetSuccessPatternsByLanguage(ctx context.Context, language string, minConfidence float64) ([]SuccessPattern, error) {
	query := `
		SELECT id, signature, language, success_rate, occurrence_count,
			   avg_duration, confidence_level, factors, conditions, created_at, updated_at
		FROM success_patterns 
		WHERE language = $1 AND confidence_level >= $2 
		ORDER BY success_rate DESC, occurrence_count DESC`

	rows, err := db.pool.Query(ctx, query, language, minConfidence)
	if err != nil {
		return nil, fmt.Errorf("failed to query success patterns: %w", err)
	}
	defer rows.Close()

	var patterns []SuccessPattern
	for rows.Next() {
		var pattern SuccessPattern
		err := rows.Scan(
			&pattern.ID, &pattern.Signature, &pattern.Language, &pattern.SuccessRate,
			&pattern.OccurrenceCount, &pattern.AvgDuration, &pattern.ConfidenceLevel,
			&pattern.Factors, &pattern.Conditions, &pattern.CreatedAt, &pattern.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan success pattern: %w", err)
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// CreateOrUpdateStrategyWeight creates or updates a strategy weight
func (db *LearningDB) CreateOrUpdateStrategyWeight(ctx context.Context, weight *StrategyWeight) (*StrategyWeight, error) {
	query := `
		INSERT INTO strategy_weights (
			strategy_name, language, pattern_type, 
			weight, performance_score, sample_size
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) 
		ON CONFLICT (strategy_name, language, pattern_type) 
		DO UPDATE SET 
			weight = $4,
			performance_score = $5,
			sample_size = $6,
			updated_at = NOW()
		RETURNING id, updated_at`

	err := db.pool.QueryRow(ctx, query,
		weight.StrategyName, weight.Language, weight.PatternType,
		weight.Weight, weight.PerformanceScore, weight.SampleSize,
	).Scan(&weight.ID, &weight.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create/update strategy weight: %w", err)
	}

	return weight, nil
}

// GetOptimalStrategies retrieves optimal strategies for a pattern type
func (db *LearningDB) GetOptimalStrategies(ctx context.Context, language, patternType string, minSampleSize int32) ([]StrategyWeight, error) {
	query := `
		SELECT id, strategy_name, language, pattern_type, weight, 
			   performance_score, sample_size, updated_at
		FROM strategy_weights 
		WHERE language = $1 AND pattern_type = $2 AND sample_size >= $3 
		ORDER BY weight DESC, performance_score DESC`

	rows, err := db.pool.Query(ctx, query, language, patternType, minSampleSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query optimal strategies: %w", err)
	}
	defer rows.Close()

	var strategies []StrategyWeight
	for rows.Next() {
		var strategy StrategyWeight
		err := rows.Scan(
			&strategy.ID, &strategy.StrategyName, &strategy.Language, &strategy.PatternType,
			&strategy.Weight, &strategy.PerformanceScore, &strategy.SampleSize, &strategy.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan strategy weight: %w", err)
		}
		strategies = append(strategies, strategy)
	}

	return strategies, nil
}
