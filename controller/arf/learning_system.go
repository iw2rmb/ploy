package arf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	
	"github.com/ploy/ploy/controller/arf/db"
)

// LearningSystem defines the interface for continuous learning and pattern extraction
type LearningSystem interface {
	RecordTransformationOutcome(ctx context.Context, outcome TransformationOutcome) error
	ExtractPatterns(ctx context.Context, timeWindow time.Duration) (*PatternAnalysis, error)
	UpdateStrategyWeights(ctx context.Context, patterns PatternAnalysis) error
	GenerateRecipeTemplate(ctx context.Context, pattern SuccessPattern) (*RecipeTemplate, error)
}

// TransformationOutcome represents the complete outcome of a transformation
type TransformationOutcome struct {
	TransformationID  string                    `json:"transformation_id"`
	Repository        RepositoryMetadata        `json:"repository"`
	Strategy          TransformationStrategy    `json:"strategy"`
	Result            TransformationResult      `json:"result"`
	Metrics           PerformanceMetrics        `json:"metrics"`
	Context           EnvironmentContext        `json:"context"`
	Timestamp         time.Time                 `json:"timestamp"`
}

// RepositoryMetadata contains metadata about the repository being transformed
type RepositoryMetadata struct {
	URL          string            `json:"url"`
	Language     string            `json:"language"`
	Framework    string            `json:"framework"`
	Size         int64             `json:"size"`
	FileCount    int               `json:"file_count"`
	Dependencies []string          `json:"dependencies"`
	TestCoverage float64           `json:"test_coverage"`
	Complexity   float64           `json:"complexity"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// EnvironmentContext captures the environment in which transformation occurred
type EnvironmentContext struct {
	PlatformVersion   string            `json:"platform_version"`
	ControllerVersion string            `json:"controller_version"`
	ResourcesUsed     ResourceUsage     `json:"resources_used"`
	ConfigurationHash string            `json:"configuration_hash"`
	NodeInfo          map[string]interface{} `json:"node_info"`
}

// ResourceUsage tracks actual resource consumption
type ResourceUsage struct {
	CPUMillis    int64 `json:"cpu_millis"`
	MemoryBytes  int64 `json:"memory_bytes"`
	DiskBytes    int64 `json:"disk_bytes"`
	NetworkBytes int64 `json:"network_bytes"`
}

// PatternAnalysis contains extracted patterns from transformation history
type PatternAnalysis struct {
	SuccessPatterns        []SuccessPattern   `json:"success_patterns"`
	FailurePatterns        []FailurePattern   `json:"failure_patterns"`
	StrategyEffectiveness  map[string]float64 `json:"strategy_effectiveness"`
	RecommendedUpdates     []StrategyUpdate   `json:"recommended_updates"`
	Confidence             float64            `json:"confidence"`
	AnalysisTimestamp      time.Time          `json:"analysis_timestamp"`
}

// SuccessPattern represents a pattern of successful transformations
type SuccessPattern struct {
	Signature         string                 `json:"signature"`
	Frequency         int                    `json:"frequency"`
	SuccessRate       float64                `json:"success_rate"`
	OptimalStrategy   TransformationStrategy `json:"optimal_strategy"`
	ContextFactors    []string               `json:"context_factors"`
	Generalization    PatternGeneralization  `json:"generalization"`
	Examples          []string               `json:"examples"`
}

// FailurePattern represents a pattern of failed transformations
type FailurePattern struct {
	Signature       string               `json:"signature"`
	Frequency       int                  `json:"frequency"`
	FailureRate     float64              `json:"failure_rate"`
	CommonErrors    []string             `json:"common_errors"`
	ContextFactors  []string             `json:"context_factors"`
	Mitigations     []string             `json:"mitigations"`
}

// PatternGeneralization describes how a pattern can be applied more broadly
type PatternGeneralization struct {
	ApplicableLanguages []string `json:"applicable_languages"`
	ApplicableFrameworks []string `json:"applicable_frameworks"`
	Conditions          []string `json:"conditions"`
	ConfidenceLevel     float64  `json:"confidence_level"`
}

// StrategyUpdate represents a recommended update to transformation strategies
type StrategyUpdate struct {
	StrategyType    StrategyType `json:"strategy_type"`
	CurrentWeight   float64      `json:"current_weight"`
	RecommendedWeight float64    `json:"recommended_weight"`
	Reasoning       string       `json:"reasoning"`
	Evidence        []string     `json:"evidence"`
}

// RecipeTemplate represents a generalized recipe template derived from patterns
type RecipeTemplate struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	ApplicablePattern SuccessPattern        `json:"applicable_pattern"`
	Template         map[string]interface{} `json:"template"`
	Variables        []TemplateVariable     `json:"variables"`
	SuccessRate      float64                `json:"success_rate"`
	UsageCount       int                    `json:"usage_count"`
	CreatedAt        time.Time              `json:"created_at"`
}

// TemplateVariable defines a variable in a recipe template
type TemplateVariable struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	DefaultValue interface{} `json:"default_value"`
	Description  string      `json:"description"`
	Required     bool        `json:"required"`
}

// PostgreSQLLearningSystem implements learning system using PostgreSQL with pgx driver
type PostgreSQLLearningSystem struct {
	db               *sql.DB
	learningDB       *db.LearningDB
	abTestFramework  ABTestFramework
	patternMatcher   PatternMatcher
	strategyWeights  map[StrategyType]float64
}

// NewPostgreSQLLearningSystem creates a new PostgreSQL-based learning system
func NewPostgreSQLLearningSystem() (*PostgreSQLLearningSystem, error) {
	dbURL := os.Getenv("ARF_LEARNING_DB_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("ARF_LEARNING_DB_URL environment variable is required")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to learning database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping learning database: %w", err)
	}

	system := &PostgreSQLLearningSystem{
		db:              db,
		abTestFramework: NewDefaultABTestFramework(db),
		patternMatcher:  NewDefaultPatternMatcher(),
		strategyWeights: make(map[StrategyType]float64),
	}

	// Initialize database schema
	if err := system.initializeSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Load current strategy weights
	if err := system.loadStrategyWeights(); err != nil {
		return nil, fmt.Errorf("failed to load strategy weights: %w", err)
	}

	return system, nil
}

// RecordTransformationOutcome records the outcome of a transformation for learning
func (ls *PostgreSQLLearningSystem) RecordTransformationOutcome(ctx context.Context, outcome TransformationOutcome) error {
	tx, err := ls.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert transformation outcome
	outcomeQuery := `
		INSERT INTO transformation_outcomes (
			id, transformation_id, repository_id, language, framework,
			recipe_id, strategy_type, success, confidence_score,
			execution_time_ms, error_classification, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	errorClassification := ""
	if !outcome.Result.Success && len(outcome.Result.Errors) > 0 {
		errorClassification = outcome.Result.Errors[0].Type
	}

	_, err = tx.ExecContext(ctx, outcomeQuery,
		generateUUID(),
		outcome.TransformationID,
		outcome.Repository.URL,
		outcome.Repository.Language,
		outcome.Repository.Framework,
		outcome.Result.RecipeID,
		string(outcome.Strategy.Primary),
		outcome.Result.Success,
		outcome.Result.ValidationScore,
		outcome.Result.ExecutionTime.Milliseconds(),
		errorClassification,
		outcome.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert transformation outcome: %w", err)
	}

	// Insert transformation features for ML
	featuresQuery := `
		INSERT INTO transformation_features (
			transformation_id, repo_size_kb, file_count, complexity_score,
			dependency_count, test_coverage, language_features,
			framework_features, outcome_label
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	languageFeatures, _ := json.Marshal(map[string]interface{}{
		"language": outcome.Repository.Language,
	})
	frameworkFeatures, _ := json.Marshal(map[string]interface{}{
		"framework": outcome.Repository.Framework,
	})

	outcomeLabel := "failure"
	if outcome.Result.Success {
		outcomeLabel = "success"
	}

	_, err = tx.ExecContext(ctx, featuresQuery,
		outcome.TransformationID,
		outcome.Repository.Size/1024, // Convert to KB
		outcome.Repository.FileCount,
		outcome.Repository.Complexity,
		len(outcome.Repository.Dependencies),
		outcome.Repository.TestCoverage,
		languageFeatures,
		frameworkFeatures,
		outcomeLabel,
	)
	if err != nil {
		return fmt.Errorf("failed to insert transformation features: %w", err)
	}

	return tx.Commit()
}

// ExtractPatterns extracts success and failure patterns from recent transformation history
func (ls *PostgreSQLLearningSystem) ExtractPatterns(ctx context.Context, timeWindow time.Duration) (*PatternAnalysis, error) {
	since := time.Now().Add(-timeWindow)

	// Extract success patterns
	successPatterns, err := ls.extractSuccessPatterns(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to extract success patterns: %w", err)
	}

	// Extract failure patterns
	failurePatterns, err := ls.extractFailurePatterns(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to extract failure patterns: %w", err)
	}

	// Calculate strategy effectiveness
	strategyEffectiveness, err := ls.calculateStrategyEffectiveness(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate strategy effectiveness: %w", err)
	}

	// Generate strategy updates
	recommendedUpdates := ls.generateStrategyUpdates(strategyEffectiveness)

	// Calculate overall confidence in the analysis
	confidence := ls.calculateAnalysisConfidence(len(successPatterns), len(failurePatterns), timeWindow)

	return &PatternAnalysis{
		SuccessPatterns:       successPatterns,
		FailurePatterns:       failurePatterns,
		StrategyEffectiveness: strategyEffectiveness,
		RecommendedUpdates:    recommendedUpdates,
		Confidence:            confidence,
		AnalysisTimestamp:     time.Now(),
	}, nil
}

// UpdateStrategyWeights updates strategy weights based on pattern analysis
func (ls *PostgreSQLLearningSystem) UpdateStrategyWeights(ctx context.Context, patterns PatternAnalysis) error {
	for _, update := range patterns.RecommendedUpdates {
		oldWeight := ls.strategyWeights[update.StrategyType]
		
		// Apply gradual weight adjustment (learning rate = 0.1)
		learningRate := 0.1
		newWeight := oldWeight + learningRate*(update.RecommendedWeight-oldWeight)
		
		ls.strategyWeights[update.StrategyType] = newWeight
		
		// Record weight update in database
		updateQuery := `
			INSERT INTO strategy_weights (strategy_type, weight, updated_at, reasoning)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (strategy_type) DO UPDATE SET
				weight = EXCLUDED.weight,
				updated_at = EXCLUDED.updated_at,
				reasoning = EXCLUDED.reasoning`
		
		_, err := ls.db.ExecContext(ctx, updateQuery,
			string(update.StrategyType),
			newWeight,
			time.Now(),
			update.Reasoning,
		)
		if err != nil {
			return fmt.Errorf("failed to update strategy weight for %s: %w", update.StrategyType, err)
		}
	}

	return nil
}

// GenerateRecipeTemplate creates a reusable recipe template from a successful pattern
func (ls *PostgreSQLLearningSystem) GenerateRecipeTemplate(ctx context.Context, pattern SuccessPattern) (*RecipeTemplate, error) {
	template := &RecipeTemplate{
		ID:                fmt.Sprintf("generated-%d", time.Now().Unix()),
		Name:              fmt.Sprintf("Template for %s", pattern.Signature),
		Description:       fmt.Sprintf("Auto-generated template based on successful pattern: %s", pattern.Signature),
		ApplicablePattern: pattern,
		SuccessRate:       pattern.SuccessRate,
		UsageCount:        0,
		CreatedAt:         time.Now(),
		Variables:         []TemplateVariable{},
	}

	// Generate template structure based on pattern
	templateMap := map[string]interface{}{
		"type":     "generated_template",
		"pattern":  pattern.Signature,
		"strategy": pattern.OptimalStrategy.Primary,
		"confidence_threshold": pattern.SuccessRate,
	}

	// Add context-specific variables
	for _, factor := range pattern.ContextFactors {
		templateMap[factor] = "${" + factor + "}"
		template.Variables = append(template.Variables, TemplateVariable{
			Name:        factor,
			Type:        "string",
			Description: fmt.Sprintf("Context factor: %s", factor),
			Required:    true,
		})
	}

	template.Template = templateMap

	// Store template in database
	templateJSON, err := json.Marshal(template.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal template: %w", err)
	}

	// Variables are part of the template, no need to store separately

	insertQuery := `
		INSERT INTO pattern_templates (
			id, pattern_signature, language, success_rate, usage_count,
			template_recipe, confidence_threshold, created_at, last_used
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = ls.db.ExecContext(ctx, insertQuery,
		template.ID,
		pattern.Signature,
		"multi", // Multi-language template
		template.SuccessRate,
		template.UsageCount,
		templateJSON,
		pattern.SuccessRate,
		template.CreatedAt,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store recipe template: %w", err)
	}

	return template, nil
}

// Helper methods for pattern extraction

func (ls *PostgreSQLLearningSystem) extractSuccessPatterns(ctx context.Context, since time.Time) ([]SuccessPattern, error) {
	query := `
		SELECT 
			CONCAT(language, '-', framework, '-', strategy_type) as signature,
			COUNT(*) as frequency,
			AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) as success_rate,
			strategy_type,
			language,
			framework
		FROM transformation_outcomes 
		WHERE created_at >= $1 AND success = true
		GROUP BY language, framework, strategy_type
		HAVING COUNT(*) >= 5 AND AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) >= 0.8
		ORDER BY success_rate DESC, frequency DESC`

	rows, err := ls.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []SuccessPattern
	for rows.Next() {
		var signature, strategyType, language, framework string
		var frequency int
		var successRate float64

		err := rows.Scan(&signature, &frequency, &successRate, &strategyType, &language, &framework)
		if err != nil {
			return nil, err
		}

		pattern := SuccessPattern{
			Signature:   signature,
			Frequency:   frequency,
			SuccessRate: successRate,
			OptimalStrategy: TransformationStrategy{
				Primary:    StrategyType(strategyType),
				Confidence: successRate,
			},
			ContextFactors: []string{language, framework},
			Generalization: PatternGeneralization{
				ApplicableLanguages:  []string{language},
				ApplicableFrameworks: []string{framework},
				ConfidenceLevel:      successRate,
			},
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

func (ls *PostgreSQLLearningSystem) extractFailurePatterns(ctx context.Context, since time.Time) ([]FailurePattern, error) {
	query := `
		SELECT 
			CONCAT(language, '-', framework, '-', error_classification) as signature,
			COUNT(*) as frequency,
			AVG(CASE WHEN success THEN 0.0 ELSE 1.0 END) as failure_rate,
			array_agg(DISTINCT error_classification) as common_errors,
			language,
			framework
		FROM transformation_outcomes 
		WHERE created_at >= $1 AND success = false
		GROUP BY language, framework, error_classification
		HAVING COUNT(*) >= 3
		ORDER BY failure_rate DESC, frequency DESC`

	rows, err := ls.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []FailurePattern
	for rows.Next() {
		var signature, language, framework string
		var frequency int
		var failureRate float64
		var commonErrorsJSON []byte

		err := rows.Scan(&signature, &frequency, &failureRate, &commonErrorsJSON, &language, &framework)
		if err != nil {
			return nil, err
		}

		var commonErrors []string
		if err := json.Unmarshal(commonErrorsJSON, &commonErrors); err != nil {
			commonErrors = []string{"parse_error"}
		}

		pattern := FailurePattern{
			Signature:      signature,
			Frequency:      frequency,
			FailureRate:    failureRate,
			CommonErrors:   commonErrors,
			ContextFactors: []string{language, framework},
			Mitigations:    ls.generateMitigations(commonErrors),
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

func (ls *PostgreSQLLearningSystem) calculateStrategyEffectiveness(ctx context.Context, since time.Time) (map[string]float64, error) {
	query := `
		SELECT 
			strategy_type,
			COUNT(*) as total_attempts,
			AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) as success_rate,
			AVG(confidence_score) as avg_confidence,
			AVG(execution_time_ms) as avg_execution_time
		FROM transformation_outcomes 
		WHERE created_at >= $1
		GROUP BY strategy_type`

	rows, err := ls.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	effectiveness := make(map[string]float64)
	for rows.Next() {
		var strategyType string
		var totalAttempts int
		var successRate, avgConfidence, avgExecutionTime float64

		err := rows.Scan(&strategyType, &totalAttempts, &successRate, &avgConfidence, &avgExecutionTime)
		if err != nil {
			return nil, err
		}

		// Calculate composite effectiveness score
		// Factors: success rate (50%), confidence (30%), speed (20%)
		speedScore := math.Max(0, 1.0-(avgExecutionTime/300000)) // Normalize against 5 minutes
		effectivenessScore := 0.5*successRate + 0.3*avgConfidence + 0.2*speedScore

		effectiveness[strategyType] = effectivenessScore
	}

	return effectiveness, nil
}

func (ls *PostgreSQLLearningSystem) generateStrategyUpdates(effectiveness map[string]float64) []StrategyUpdate {
	var updates []StrategyUpdate

	for strategyStr, score := range effectiveness {
		strategyType := StrategyType(strategyStr)
		currentWeight := ls.strategyWeights[strategyType]

		// Recommend weight adjustment based on effectiveness
		var recommendedWeight float64
		var reasoning string

		if score > 0.8 {
			recommendedWeight = math.Min(1.0, currentWeight+0.1)
			reasoning = "High effectiveness, increase weight"
		} else if score < 0.5 {
			recommendedWeight = math.Max(0.1, currentWeight-0.1)
			reasoning = "Low effectiveness, decrease weight"
		} else {
			recommendedWeight = currentWeight
			reasoning = "Stable performance, maintain weight"
		}

		if math.Abs(recommendedWeight-currentWeight) > 0.05 {
			updates = append(updates, StrategyUpdate{
				StrategyType:      strategyType,
				CurrentWeight:     currentWeight,
				RecommendedWeight: recommendedWeight,
				Reasoning:         reasoning,
				Evidence: []string{
					fmt.Sprintf("Effectiveness score: %.2f", score),
				},
			})
		}
	}

	return updates
}

func (ls *PostgreSQLLearningSystem) calculateAnalysisConfidence(successPatterns, failurePatterns int, timeWindow time.Duration) float64 {
	// Base confidence on amount of data available
	totalPatterns := successPatterns + failurePatterns
	dataConfidence := math.Min(1.0, float64(totalPatterns)/20.0) // 20 patterns = full confidence

	// Time window factor (longer windows = more confidence)
	timeConfidence := math.Min(1.0, timeWindow.Hours()/(7*24)) // 1 week = full confidence

	// Combine factors
	return 0.7*dataConfidence + 0.3*timeConfidence
}

func (ls *PostgreSQLLearningSystem) generateMitigations(commonErrors []string) []string {
	mitigationMap := map[string]string{
		"compilation_failure": "Review dependency versions and build configuration",
		"timeout":            "Increase resource allocation or break into smaller tasks",
		"memory_error":       "Increase memory limits or optimize transformation approach",
		"syntax_error":       "Improve recipe validation and syntax checking",
		"semantic_error":     "Enhance semantic analysis and context awareness",
	}

	var mitigations []string
	for _, errorType := range commonErrors {
		if mitigation, exists := mitigationMap[errorType]; exists {
			mitigations = append(mitigations, mitigation)
		}
	}

	if len(mitigations) == 0 {
		mitigations = []string{"Review and adjust transformation parameters"}
	}

	return mitigations
}

func (ls *PostgreSQLLearningSystem) initializeSchema() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS transformation_outcomes (
			id UUID PRIMARY KEY,
			transformation_id UUID NOT NULL,
			repository_id UUID NOT NULL,
			language VARCHAR(50),
			framework VARCHAR(100),
			recipe_id UUID,
			strategy_type VARCHAR(50),
			success BOOLEAN,
			confidence_score FLOAT,
			execution_time_ms INT,
			error_classification VARCHAR(100),
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS pattern_templates (
			id UUID PRIMARY KEY,
			pattern_signature TEXT,
			language VARCHAR(50),
			success_rate FLOAT,
			usage_count INT,
			template_recipe JSONB,
			confidence_threshold FLOAT,
			created_at TIMESTAMP DEFAULT NOW(),
			last_used TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ab_experiments (
			id UUID PRIMARY KEY,
			experiment_name VARCHAR(200),
			variant_a_recipe JSONB,
			variant_b_recipe JSONB,
			variant_a_count INT DEFAULT 0,
			variant_b_count INT DEFAULT 0,
			variant_a_success_rate FLOAT,
			variant_b_success_rate FLOAT,
			statistical_significance FLOAT,
			status VARCHAR(50),
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS transformation_features (
			transformation_id UUID PRIMARY KEY,
			repo_size_kb INT,
			file_count INT,
			complexity_score FLOAT,
			dependency_count INT,
			test_coverage FLOAT,
			language_features JSONB,
			framework_features JSONB,
			outcome_label VARCHAR(50)
		)`,
		`CREATE TABLE IF NOT EXISTS strategy_weights (
			strategy_type VARCHAR(50) PRIMARY KEY,
			weight FLOAT NOT NULL,
			updated_at TIMESTAMP DEFAULT NOW(),
			reasoning TEXT
		)`,
	}

	for _, schema := range schemas {
		if _, err := ls.db.Exec(schema); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	return nil
}

func (ls *PostgreSQLLearningSystem) loadStrategyWeights() error {
	query := `SELECT strategy_type, weight FROM strategy_weights`
	rows, err := ls.db.Query(query)
	if err != nil {
		// Initialize default weights if table is empty
		ls.strategyWeights = map[StrategyType]float64{
			StrategyOpenRewriteOnly: 0.7,
			StrategyLLMOnly:        0.5,
			StrategyHybridSequential: 0.8,
			StrategyHybridParallel:  0.6,
			StrategyTreeSitter:     0.6,
		}
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var strategyType string
		var weight float64
		if err := rows.Scan(&strategyType, &weight); err != nil {
			return err
		}
		ls.strategyWeights[StrategyType(strategyType)] = weight
	}

	return nil
}

// Utility functions
func generateUUID() string {
	// Simple UUID generation - in production use proper UUID library
	return fmt.Sprintf("arf-%d-%d", time.Now().UnixNano(), time.Now().Unix())
}