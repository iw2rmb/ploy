package arf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/iw2rmb/ploy/controller/arf/models"
)

// ABTestFramework defines the interface for A/B testing recipe variations
type ABTestFramework interface {
	CreateExperiment(ctx context.Context, experiment ABExperiment) error
	SelectVariant(ctx context.Context, experimentID string) (*Variant, error)
	RecordOutcome(ctx context.Context, variantID string, success bool) error
	AnalyzeResults(ctx context.Context, experimentID string) (*ABTestResults, error)
	GraduateWinner(ctx context.Context, experimentID string) error
}

// ABExperiment defines an A/B test experiment
type ABExperiment struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	VariantA        *models.Recipe  `json:"variant_a"`
	VariantB        *models.Recipe  `json:"variant_b"`
	TrafficSplit    float64         `json:"traffic_split"`
	MinSampleSize   int             `json:"min_sample_size"`
	ConfidenceLevel float64         `json:"confidence_level"`
}

// Variant represents a recipe variant in an experiment
type Variant struct {
	ID         string  `json:"id"`
	Recipe     *models.Recipe `json:"recipe"`
	Weight     float64 `json:"weight"`
	IsControl  bool    `json:"is_control"`
}

// ABTestResults contains the statistical analysis of an A/B test
type ABTestResults struct {
	ExperimentID     string                 `json:"experiment_id"`
	VariantAResults  VariantResults         `json:"variant_a_results"`
	VariantBResults  VariantResults         `json:"variant_b_results"`
	StatisticalTest  StatisticalTestResult  `json:"statistical_test"`
	Recommendation   ABTestRecommendation   `json:"recommendation"`
	AnalyzedAt       time.Time              `json:"analyzed_at"`
}

// VariantResults contains results for a specific variant
type VariantResults struct {
	VariantID       string    `json:"variant_id"`
	TotalTrials     int       `json:"total_trials"`
	Successes       int       `json:"successes"`
	SuccessRate     float64   `json:"success_rate"`
	ConfidenceInterval ConfidenceInterval `json:"confidence_interval"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
}

// ConfidenceInterval represents a statistical confidence interval
type ConfidenceInterval struct {
	Lower      float64 `json:"lower"`
	Upper      float64 `json:"upper"`
	Confidence float64 `json:"confidence"`
}

// StatisticalTestResult contains the results of statistical significance testing
type StatisticalTestResult struct {
	TestType     string  `json:"test_type"`
	PValue       float64 `json:"p_value"`
	ZScore       float64 `json:"z_score"`
	Significant  bool    `json:"significant"`
	EffectSize   float64 `json:"effect_size"`
	PowerAnalysis PowerAnalysis `json:"power_analysis"`
}

// PowerAnalysis contains statistical power analysis
type PowerAnalysis struct {
	Power              float64 `json:"power"`
	MinDetectableDiff  float64 `json:"min_detectable_diff"`
	RecommendedSample  int     `json:"recommended_sample"`
}

// ABTestRecommendation provides guidance based on test results
type ABTestRecommendation struct {
	Action       string  `json:"action"` // "adopt_a", "adopt_b", "continue_test", "inconclusive"
	Confidence   float64 `json:"confidence"`
	WinningVariant string `json:"winning_variant"`
	Reasoning    string  `json:"reasoning"`
	NextSteps    []string `json:"next_steps"`
}

// DefaultABTestFramework implements A/B testing using PostgreSQL
type DefaultABTestFramework struct {
	db *sql.DB
}

// NewDefaultABTestFramework creates a new A/B test framework
func NewDefaultABTestFramework(db *sql.DB) *DefaultABTestFramework {
	return &DefaultABTestFramework{
		db: db,
	}
}

// CreateExperiment creates a new A/B test experiment
func (f *DefaultABTestFramework) CreateExperiment(ctx context.Context, experiment ABExperiment) error {
	variantAJSON, err := json.Marshal(experiment.VariantA)
	if err != nil {
		return fmt.Errorf("failed to marshal variant A: %w", err)
	}

	variantBJSON, err := json.Marshal(experiment.VariantB)
	if err != nil {
		return fmt.Errorf("failed to marshal variant B: %w", err)
	}

	query := `
		INSERT INTO ab_experiments (
			id, experiment_name, variant_a_recipe, variant_b_recipe,
			variant_a_count, variant_b_count, variant_a_success_rate,
			variant_b_success_rate, statistical_significance, status, created_at
		) VALUES ($1, $2, $3, $4, 0, 0, 0.0, 0.0, 0.0, 'active', $5)`

	_, err = f.db.ExecContext(ctx, query,
		experiment.ID,
		experiment.Name,
		variantAJSON,
		variantBJSON,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create experiment: %w", err)
	}

	return nil
}

// SelectVariant selects a variant for testing based on traffic split
func (f *DefaultABTestFramework) SelectVariant(ctx context.Context, experimentID string) (*Variant, error) {
	// Get experiment details
	query := `SELECT variant_a_recipe, variant_b_recipe, variant_a_count, variant_b_count 
	         FROM ab_experiments WHERE id = $1 AND status = 'active'`

	var variantAJSON, variantBJSON []byte
	var countA, countB int

	err := f.db.QueryRowContext(ctx, query, experimentID).Scan(&variantAJSON, &variantBJSON, &countA, &countB)
	if err != nil {
		return nil, fmt.Errorf("failed to get experiment: %w", err)
	}

	// Simple traffic split logic (50/50 for now)
	totalCount := countA + countB
	useVariantA := totalCount%2 == 0

	if useVariantA {
		var recipe models.Recipe
		if err := json.Unmarshal(variantAJSON, &recipe); err != nil {
			return nil, fmt.Errorf("failed to unmarshal variant A: %w", err)
		}

		return &Variant{
			ID:        experimentID + "-variant-a",
			Recipe:    &recipe,
			Weight:    0.5,
			IsControl: true,
		}, nil
	} else {
		var recipe models.Recipe
		if err := json.Unmarshal(variantBJSON, &recipe); err != nil {
			return nil, fmt.Errorf("failed to unmarshal variant B: %w", err)
		}

		return &Variant{
			ID:        experimentID + "-variant-b",
			Recipe:    &recipe,
			Weight:    0.5,
			IsControl: false,
		}, nil
	}
}

// RecordOutcome records the outcome of a variant test
func (f *DefaultABTestFramework) RecordOutcome(ctx context.Context, variantID string, success bool) error {
	// Determine which variant this is
	isVariantA := len(variantID) > 0 && variantID[len(variantID)-1] == 'a'

	var updateQuery string
	if isVariantA {
		updateQuery = `
			UPDATE ab_experiments SET 
				variant_a_count = variant_a_count + 1,
				variant_a_success_rate = (
					SELECT CASE 
						WHEN variant_a_count + 1 = 0 THEN 0.0
						ELSE (variant_a_success_rate * variant_a_count + CASE WHEN $2 THEN 1.0 ELSE 0.0 END) / (variant_a_count + 1)
					END
				)
			WHERE id = $1`
	} else {
		updateQuery = `
			UPDATE ab_experiments SET 
				variant_b_count = variant_b_count + 1,
				variant_b_success_rate = (
					SELECT CASE 
						WHEN variant_b_count + 1 = 0 THEN 0.0
						ELSE (variant_b_success_rate * variant_b_count + CASE WHEN $2 THEN 1.0 ELSE 0.0 END) / (variant_b_count + 1)
					END
				)
			WHERE id = $1`
	}

	// Extract experiment ID from variant ID
	experimentID := variantID[:len(variantID)-10] // Remove "-variant-a/b"

	_, err := f.db.ExecContext(ctx, updateQuery, experimentID, success)
	if err != nil {
		return fmt.Errorf("failed to record outcome: %w", err)
	}

	return nil
}

// AnalyzeResults performs statistical analysis of A/B test results
func (f *DefaultABTestFramework) AnalyzeResults(ctx context.Context, experimentID string) (*ABTestResults, error) {
	// Get experiment data
	query := `SELECT variant_a_count, variant_b_count, variant_a_success_rate, variant_b_success_rate 
	         FROM ab_experiments WHERE id = $1`

	var countA, countB int
	var successRateA, successRateB float64

	err := f.db.QueryRowContext(ctx, query, experimentID).Scan(&countA, &countB, &successRateA, &successRateB)
	if err != nil {
		return nil, fmt.Errorf("failed to get experiment data: %w", err)
	}

	// Calculate variant results
	variantAResults := VariantResults{
		VariantID:         experimentID + "-variant-a",
		TotalTrials:       countA,
		Successes:         int(float64(countA) * successRateA),
		SuccessRate:       successRateA,
		ConfidenceInterval: f.calculateConfidenceInterval(successRateA, countA, 0.95),
	}

	variantBResults := VariantResults{
		VariantID:         experimentID + "-variant-b", 
		TotalTrials:       countB,
		Successes:         int(float64(countB) * successRateB),
		SuccessRate:       successRateB,
		ConfidenceInterval: f.calculateConfidenceInterval(successRateB, countB, 0.95),
	}

	// Perform statistical test
	statisticalTest := f.performZTest(variantAResults, variantBResults)

	// Generate recommendation
	recommendation := f.generateRecommendation(variantAResults, variantBResults, statisticalTest)

	// Update statistical significance in database
	_, err = f.db.ExecContext(ctx, 
		`UPDATE ab_experiments SET statistical_significance = $1 WHERE id = $2`,
		statisticalTest.PValue, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to update statistical significance: %w", err)
	}

	return &ABTestResults{
		ExperimentID:    experimentID,
		VariantAResults: variantAResults,
		VariantBResults: variantBResults,
		StatisticalTest: statisticalTest,
		Recommendation:  recommendation,
		AnalyzedAt:      time.Now(),
	}, nil
}

// GraduateWinner promotes the winning variant and ends the experiment
func (f *DefaultABTestFramework) GraduateWinner(ctx context.Context, experimentID string) error {
	// Analyze results first
	results, err := f.AnalyzeResults(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("failed to analyze results: %w", err)
	}

	if !results.StatisticalTest.Significant {
		return fmt.Errorf("cannot graduate winner: results not statistically significant")
	}

	// Mark experiment as completed
	_, err = f.db.ExecContext(ctx,
		`UPDATE ab_experiments SET status = 'completed' WHERE id = $1`,
		experimentID)
	if err != nil {
		return fmt.Errorf("failed to complete experiment: %w", err)
	}

	// The winning recipe would be promoted in the recipe management system
	// This is a placeholder for that integration
	fmt.Printf("A/B Test %s completed. Winner: %s with %.2f%% success rate\n",
		experimentID, results.Recommendation.WinningVariant,
		f.getWinningSuccessRate(results)*100)

	return nil
}

// Helper methods for statistical calculations

func (f *DefaultABTestFramework) calculateConfidenceInterval(successRate float64, sampleSize int, confidence float64) ConfidenceInterval {
	if sampleSize == 0 {
		return ConfidenceInterval{Lower: 0, Upper: 1, Confidence: confidence}
	}

	// Wilson score interval for better small sample performance
	z := 1.96 // 95% confidence
	if confidence == 0.90 {
		z = 1.645
	} else if confidence == 0.99 {
		z = 2.576
	}

	n := float64(sampleSize)
	p := successRate

	denominator := 1 + (z*z)/n
	center := p + (z*z)/(2*n)
	spread := z * math.Sqrt((p*(1-p))/n + (z*z)/(4*n*n))

	lower := (center - spread) / denominator
	upper := (center + spread) / denominator

	return ConfidenceInterval{
		Lower:      math.Max(0, lower),
		Upper:      math.Min(1, upper),
		Confidence: confidence,
	}
}

func (f *DefaultABTestFramework) performZTest(variantA, variantB VariantResults) StatisticalTestResult {
	// Two-proportion Z-test
	p1 := variantA.SuccessRate
	p2 := variantB.SuccessRate
	n1 := float64(variantA.TotalTrials)
	n2 := float64(variantB.TotalTrials)

	if n1 == 0 || n2 == 0 {
		return StatisticalTestResult{
			TestType:    "two_proportion_z_test",
			PValue:      1.0,
			ZScore:      0.0,
			Significant: false,
			EffectSize:  0.0,
		}
	}

	// Pooled proportion
	pooledP := (p1*n1 + p2*n2) / (n1 + n2)
	
	// Standard error
	se := math.Sqrt(pooledP * (1 - pooledP) * (1/n1 + 1/n2))
	
	// Z-score
	zScore := 0.0
	if se > 0 {
		zScore = (p1 - p2) / se
	}

	// Two-tailed p-value
	pValue := 2 * (1 - f.normalCDF(math.Abs(zScore)))

	// Effect size (Cohen's h for proportions)
	effectSize := 2 * (math.Asin(math.Sqrt(p1)) - math.Asin(math.Sqrt(p2)))

	// Statistical significance (alpha = 0.05)
	significant := pValue < 0.05

	// Power analysis
	powerAnalysis := f.calculatePowerAnalysis(n1, n2, p1, p2)

	return StatisticalTestResult{
		TestType:      "two_proportion_z_test",
		PValue:        pValue,
		ZScore:        zScore,
		Significant:   significant,
		EffectSize:    effectSize,
		PowerAnalysis: powerAnalysis,
	}
}

func (f *DefaultABTestFramework) normalCDF(x float64) float64 {
	// Approximation of the standard normal CDF
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func (f *DefaultABTestFramework) calculatePowerAnalysis(n1, n2, p1, p2 float64) PowerAnalysis {
	// Simplified power analysis
	effectSize := math.Abs(p1 - p2)
	
	// Approximate power calculation
	power := 0.8 // Placeholder
	if effectSize > 0.1 && n1 > 30 && n2 > 30 {
		power = 0.9
	} else if effectSize > 0.05 && n1 > 50 && n2 > 50 {
		power = 0.8
	} else {
		power = 0.6
	}

	// Minimum detectable difference with 80% power
	minDetectableDiff := 0.05 // 5% difference
	if n1 < 30 || n2 < 30 {
		minDetectableDiff = 0.1 // 10% difference for smaller samples
	}

	// Recommended sample size for 80% power to detect 5% difference
	recommendedSample := 100
	if effectSize < 0.05 {
		recommendedSample = 200
	}

	return PowerAnalysis{
		Power:             power,
		MinDetectableDiff: minDetectableDiff,
		RecommendedSample: recommendedSample,
	}
}

func (f *DefaultABTestFramework) generateRecommendation(variantA, variantB VariantResults, test StatisticalTestResult) ABTestRecommendation {
	minSampleSize := 30
	totalSamples := variantA.TotalTrials + variantB.TotalTrials

	// Not enough data
	if totalSamples < minSampleSize {
		return ABTestRecommendation{
			Action:       "continue_test",
			Confidence:   0.0,
			Reasoning:    fmt.Sprintf("Insufficient data: %d samples (need at least %d)", totalSamples, minSampleSize),
			NextSteps:    []string{"Continue collecting data", "Aim for at least 30 samples per variant"},
		}
	}

	// Not statistically significant
	if !test.Significant {
		return ABTestRecommendation{
			Action:     "inconclusive",
			Confidence: 1.0 - test.PValue,
			Reasoning:  fmt.Sprintf("No statistically significant difference (p=%.3f)", test.PValue),
			NextSteps:  []string{"Consider longer test period", "Analyze practical significance", "Use variant A (control) as default"},
		}
	}

	// Statistically significant - determine winner
	var winningVariant string
	var action string
	if variantA.SuccessRate > variantB.SuccessRate {
		winningVariant = "variant_a"
		action = "adopt_a"
	} else {
		winningVariant = "variant_b"
		action = "adopt_b"
	}

	confidence := 1.0 - test.PValue
	reasoning := fmt.Sprintf("Statistically significant improvement (p=%.3f, effect size=%.3f)",
		test.PValue, test.EffectSize)

	nextSteps := []string{
		fmt.Sprintf("Deploy %s to production", winningVariant),
		"Monitor performance metrics",
		"Document learnings for future experiments",
	}

	return ABTestRecommendation{
		Action:         action,
		Confidence:     confidence,
		WinningVariant: winningVariant,
		Reasoning:      reasoning,
		NextSteps:      nextSteps,
	}
}

func (f *DefaultABTestFramework) getWinningSuccessRate(results *ABTestResults) float64 {
	if results.Recommendation.WinningVariant == "variant_a" {
		return results.VariantAResults.SuccessRate
	}
	return results.VariantBResults.SuccessRate
}

// PatternMatcher defines interface for pattern matching
type PatternMatcher interface {
	MatchPattern(pattern string, text string) bool
	ExtractPatterns(text string) []string
	GenerateSignature(context map[string]interface{}) string
}

// DefaultPatternMatcher implements basic pattern matching
type DefaultPatternMatcher struct{}

// NewDefaultPatternMatcher creates a new pattern matcher
func NewDefaultPatternMatcher() *DefaultPatternMatcher {
	return &DefaultPatternMatcher{}
}

func (m *DefaultPatternMatcher) MatchPattern(pattern string, text string) bool {
	// Simple substring matching - could be enhanced with regex
	return len(pattern) > 0 && len(text) > 0 && 
		   (pattern == text || (len(pattern) < len(text) && 
		   text[:len(pattern)] == pattern))
}

func (m *DefaultPatternMatcher) ExtractPatterns(text string) []string {
	// Simple pattern extraction - could be enhanced with ML
	return []string{text}
}

func (m *DefaultPatternMatcher) GenerateSignature(context map[string]interface{}) string {
	// Generate a signature from context
	signature := ""
	if lang, ok := context["language"]; ok {
		signature += fmt.Sprintf("%v-", lang)
	}
	if framework, ok := context["framework"]; ok {
		signature += fmt.Sprintf("%v-", framework)
	}
	if errorType, ok := context["error_type"]; ok {
		signature += fmt.Sprintf("%v", errorType)
	}
	
	if signature == "" {
		signature = "unknown"
	}
	
	return signature
}