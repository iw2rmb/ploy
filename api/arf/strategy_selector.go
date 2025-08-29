package arf

import (
	"context"
	"fmt"
	"time"
)

// StrategySelector defines the interface for selecting optimal transformation strategies
type StrategySelector interface {
	SelectStrategy(ctx context.Context, request StrategyRequest) (*SelectedStrategy, error)
	EvaluateComplexity(ctx context.Context, repository Repository) (*ComplexityAnalysis, error)
	PredictResourceRequirements(ctx context.Context, strategy TransformationStrategy) (*ResourcePrediction, error)
	RecommendEscalation(ctx context.Context, failures []TransformationFailure) (*EscalationRecommendation, error)
}

// StrategyRequest contains information needed for strategy selection
type StrategyRequest struct {
	Repository          Repository          `json:"repository"`
	TransformationType  TransformationType  `json:"transformation_type"`
	ErrorContext        ErrorContext        `json:"error_context"`
	ResourceConstraints ResourceConstraints `json:"resource_constraints"`
	TimeConstraints     TimeConstraints     `json:"time_constraints"`
	QualityRequirements QualityRequirements `json:"quality_requirements"`
}

// SelectedStrategy represents the chosen strategy with alternatives
type SelectedStrategy struct {
	Primary          TransformationStrategy   `json:"primary"`
	Alternatives     []TransformationStrategy `json:"alternatives"`
	Confidence       float64                  `json:"confidence"`
	Reasoning        StrategyReasoning        `json:"reasoning"`
	ResourceEstimate ResourcePrediction       `json:"resource_estimate"`
	TimeEstimate     time.Duration            `json:"time_estimate"`
	RiskAssessment   StrategyRiskAssessment   `json:"risk_assessment"`
}

// StrategyReasoning explains why a strategy was selected
type StrategyReasoning struct {
	PrimaryFactors    []string `json:"primary_factors"`
	ComplexityScore   float64  `json:"complexity_score"`
	HistoricalData    string   `json:"historical_data"`
	ConstraintFactors []string `json:"constraint_factors"`
	Explanation       string   `json:"explanation"`
}

// ResourcePrediction estimates resource requirements for a strategy
type ResourcePrediction struct {
	EstimatedCPU    int           `json:"estimated_cpu"`
	EstimatedMemory int64         `json:"estimated_memory"`
	EstimatedTime   time.Duration `json:"estimated_time"`
	EstimatedCost   float64       `json:"estimated_cost"`
	Confidence      float64       `json:"confidence"`
}

// StrategyRiskAssessment evaluates the risks of a transformation strategy
type StrategyRiskAssessment struct {
	OverallRisk        float64      `json:"overall_risk"`
	RiskFactors        []RiskFactor `json:"risk_factors"`
	MitigationSteps    []string     `json:"mitigation_steps"`
	FailureProbability float64      `json:"failure_probability"`
}

// RiskFactor represents a specific risk in transformation
type RiskFactor struct {
	Type        string  `json:"type"`
	Severity    float64 `json:"severity"`
	Probability float64 `json:"probability"`
	Description string  `json:"description"`
	Mitigation  string  `json:"mitigation"`
}

// Note: TransformationFailure type is defined in recipe_evolution.go

// EscalationRecommendation suggests next steps after failures
type EscalationRecommendation struct {
	RecommendedAction string                  `json:"recommended_action"`
	NextStrategy      *TransformationStrategy `json:"next_strategy"`
	HumanReview       bool                    `json:"human_review"`
	Reasoning         string                  `json:"reasoning"`
	Priority          string                  `json:"priority"`
}

// DefaultStrategySelector implements strategy selection logic
type DefaultStrategySelector struct {
	complexityAnalyzer ComplexityAnalyzer
	learningSystem     LearningSystem
	strategyWeights    map[StrategyType]float64
}

// NewDefaultStrategySelector creates a new strategy selector
func NewDefaultStrategySelector() *DefaultStrategySelector {
	return &DefaultStrategySelector{
		complexityAnalyzer: NewDefaultComplexityAnalyzer(),
		strategyWeights: map[StrategyType]float64{
			StrategyOpenRewriteOnly:  0.7,
			StrategyLLMOnly:          0.5,
			StrategyHybridSequential: 0.8,
			StrategyHybridParallel:   0.6,
			StrategyTreeSitter:       0.6,
		},
	}
}

// SelectStrategy selects the optimal transformation strategy
func (s *DefaultStrategySelector) SelectStrategy(ctx context.Context, request StrategyRequest) (*SelectedStrategy, error) {
	// Analyze complexity
	complexity, err := s.EvaluateComplexity(ctx, request.Repository)
	if err != nil {
		return nil, fmt.Errorf("complexity evaluation failed: %w", err)
	}

	// Generate candidate strategies
	candidates := s.generateCandidateStrategies(request, *complexity)

	// Score and rank strategies
	scoredStrategies := s.scoreStrategies(candidates, request, *complexity)

	// Select primary strategy
	primary := scoredStrategies[0]

	// Generate alternatives
	alternatives := scoredStrategies[1:]
	if len(alternatives) > 3 {
		alternatives = alternatives[:3] // Limit to top 3 alternatives
	}

	// Predict resource requirements
	resourceEstimate, err := s.PredictResourceRequirements(ctx, primary.strategy)
	if err != nil {
		return nil, fmt.Errorf("resource prediction failed: %w", err)
	}

	// Assess risks
	riskAssessment := s.assessRisks(primary.strategy, request, *complexity)

	// Build reasoning
	reasoning := s.buildReasoning(primary, request, *complexity)

	return &SelectedStrategy{
		Primary:          primary.strategy,
		Alternatives:     s.extractStrategies(alternatives),
		Confidence:       primary.score,
		Reasoning:        reasoning,
		ResourceEstimate: *resourceEstimate,
		TimeEstimate:     s.estimateExecutionTime(primary.strategy, *complexity),
		RiskAssessment:   riskAssessment,
	}, nil
}

// EvaluateComplexity analyzes the complexity of a repository for transformation
func (s *DefaultStrategySelector) EvaluateComplexity(ctx context.Context, repository Repository) (*ComplexityAnalysis, error) {
	return s.complexityAnalyzer.AnalyzeComplexity(ctx, repository)
}

// PredictResourceRequirements predicts the resources needed for a strategy
func (s *DefaultStrategySelector) PredictResourceRequirements(ctx context.Context, strategy TransformationStrategy) (*ResourcePrediction, error) {
	// Base predictions on strategy type
	basePrediction := s.getBaseResourcePrediction(strategy.Primary)

	// Adjust based on strategy enhancement
	if strategy.Enhancement == StrategyLLMOnly || strategy.Enhancement == StrategyHybridSequential {
		basePrediction.EstimatedCPU += 1000                  // Additional CPU for LLM processing
		basePrediction.EstimatedMemory += 1024 * 1024 * 1024 // Additional 1GB memory
		basePrediction.EstimatedTime += 30 * time.Second
		basePrediction.EstimatedCost += 0.10 // LLM API costs
	}

	return &basePrediction, nil
}

// RecommendEscalation recommends next steps after transformation failures
func (s *DefaultStrategySelector) RecommendEscalation(ctx context.Context, failures []TransformationFailure) (*EscalationRecommendation, error) {
	if len(failures) == 0 {
		return nil, fmt.Errorf("no failures provided for escalation analysis")
	}

	// Analyze failure patterns
	failureTypes := make(map[string]int)
	strategiesTried := make(map[StrategyType]bool)

	for _, failure := range failures {
		failureTypes[failure.ErrorMessage]++
		strategiesTried[StrategyType(failure.RecipeID)] = true
	}

	// Determine escalation based on patterns
	if len(failures) >= 3 {
		return &EscalationRecommendation{
			RecommendedAction: "human_review",
			HumanReview:       true,
			Reasoning:         "Multiple transformation attempts failed, human intervention required",
			Priority:          "high",
		}, nil
	}

	// Try alternative strategy
	nextStrategy := s.selectAlternativeStrategy(strategiesTried)
	if nextStrategy != nil {
		return &EscalationRecommendation{
			RecommendedAction: "retry_alternative",
			NextStrategy:      nextStrategy,
			HumanReview:       false,
			Reasoning:         fmt.Sprintf("Try alternative strategy: %s", nextStrategy.Primary),
			Priority:          "medium",
		}, nil
	}

	// Default escalation
	return &EscalationRecommendation{
		RecommendedAction: "manual_review",
		HumanReview:       true,
		Reasoning:         "All automated strategies exhausted",
		Priority:          "medium",
	}, nil
}

// Helper methods

type scoredStrategy struct {
	strategy TransformationStrategy
	score    float64
	factors  map[string]float64
}

func (s *DefaultStrategySelector) generateCandidateStrategies(request StrategyRequest, complexity ComplexityAnalysis) []TransformationStrategy {
	var candidates []TransformationStrategy

	// Always include primary strategies
	candidates = append(candidates, TransformationStrategy{
		Primary:    StrategyOpenRewriteOnly,
		Confidence: s.strategyWeights[StrategyOpenRewriteOnly],
	})

	// Add LLM strategy if complexity is high or OpenRewrite alone might not suffice
	if complexity.OverallComplexity > 0.7 {
		candidates = append(candidates, TransformationStrategy{
			Primary:    StrategyLLMOnly,
			Confidence: s.strategyWeights[StrategyLLMOnly],
		})

		candidates = append(candidates, TransformationStrategy{
			Primary:    StrategyHybridSequential,
			Confidence: s.strategyWeights[StrategyHybridSequential],
		})
	}

	// Add tree-sitter for multi-language scenarios
	if s.isMultiLanguageScenario(request) {
		candidates = append(candidates, TransformationStrategy{
			Primary:    StrategyTreeSitter,
			Confidence: s.strategyWeights[StrategyTreeSitter],
		})
	}

	// Add parallel strategy for time-sensitive requests
	if request.TimeConstraints.PreferredSpeed == "fast" {
		candidates = append(candidates, TransformationStrategy{
			Primary:    StrategyHybridParallel,
			Confidence: s.strategyWeights[StrategyHybridParallel],
		})
	}

	return candidates
}

func (s *DefaultStrategySelector) scoreStrategies(candidates []TransformationStrategy, request StrategyRequest, complexity ComplexityAnalysis) []scoredStrategy {
	var scored []scoredStrategy

	for _, candidate := range candidates {
		score := s.calculateStrategyScore(candidate, request, complexity)
		factors := s.getScoreFactors(candidate, request, complexity)

		scored = append(scored, scoredStrategy{
			strategy: candidate,
			score:    score,
			factors:  factors,
		})
	}

	// Sort by score (highest first)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[i].score < scored[j].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored
}

func (s *DefaultStrategySelector) calculateStrategyScore(strategy TransformationStrategy, request StrategyRequest, complexity ComplexityAnalysis) float64 {
	baseScore := s.strategyWeights[strategy.Primary]

	// Adjust based on complexity
	if complexity.OverallComplexity > 0.8 {
		if strategy.Primary == StrategyLLMOnly || strategy.Primary == StrategyHybridSequential {
			baseScore += 0.2 // Boost LLM strategies for complex cases
		}
	} else if complexity.OverallComplexity < 0.3 {
		if strategy.Primary == StrategyOpenRewriteOnly {
			baseScore += 0.1 // Boost simple strategy for simple cases
		}
	}

	// Adjust based on time constraints
	if request.TimeConstraints.PreferredSpeed == "fast" {
		if strategy.Primary == StrategyHybridParallel {
			baseScore += 0.15
		} else if strategy.Primary == StrategyLLMOnly {
			baseScore -= 0.1 // LLM can be slower
		}
	}

	// Adjust based on quality requirements
	if request.QualityRequirements.MinConfidence > 0.85 {
		if strategy.Primary == StrategyHybridSequential {
			baseScore += 0.1 // Hybrid often has higher confidence
		}
	}

	// Adjust based on resource constraints
	if request.ResourceConstraints.MaxMemory > 0 && request.ResourceConstraints.MaxMemory < 2*1024*1024*1024 {
		// Memory constrained, prefer lighter strategies
		if strategy.Primary == StrategyOpenRewriteOnly {
			baseScore += 0.05
		} else if strategy.Primary == StrategyLLMOnly {
			baseScore -= 0.1
		}
	}

	return baseScore
}

func (s *DefaultStrategySelector) getScoreFactors(strategy TransformationStrategy, request StrategyRequest, complexity ComplexityAnalysis) map[string]float64 {
	return map[string]float64{
		"base_weight":         s.strategyWeights[strategy.Primary],
		"complexity_factor":   complexity.OverallComplexity,
		"time_preference":     s.getTimePreferenceScore(request.TimeConstraints.PreferredSpeed),
		"quality_requirement": request.QualityRequirements.MinConfidence,
	}
}

func (s *DefaultStrategySelector) getTimePreferenceScore(preference string) float64 {
	switch preference {
	case "fast":
		return 0.8
	case "balanced":
		return 0.6
	case "thorough":
		return 0.4
	default:
		return 0.5
	}
}

func (s *DefaultStrategySelector) isMultiLanguageScenario(request StrategyRequest) bool {
	// Simple heuristic - could be enhanced with actual analysis
	return request.Repository.Language == "mixed" ||
		request.TransformationType == "migration" &&
			(request.Repository.Metadata["framework"] == "polyglot" || request.Repository.Metadata["framework"] == "microservices")
}

func (s *DefaultStrategySelector) extractStrategies(scored []scoredStrategy) []TransformationStrategy {
	var strategies []TransformationStrategy
	for _, s := range scored {
		strategies = append(strategies, s.strategy)
	}
	return strategies
}

func (s *DefaultStrategySelector) getBaseResourcePrediction(strategyType StrategyType) ResourcePrediction {
	predictions := map[StrategyType]ResourcePrediction{
		StrategyOpenRewriteOnly: {
			EstimatedCPU:    1000,
			EstimatedMemory: 1024 * 1024 * 1024, // 1GB
			EstimatedTime:   2 * time.Minute,
			EstimatedCost:   0.05,
			Confidence:      0.8,
		},
		StrategyLLMOnly: {
			EstimatedCPU:    500,
			EstimatedMemory: 512 * 1024 * 1024, // 512MB
			EstimatedTime:   30 * time.Second,
			EstimatedCost:   0.20,
			Confidence:      0.6,
		},
		StrategyHybridSequential: {
			EstimatedCPU:    1500,
			EstimatedMemory: 1536 * 1024 * 1024, // 1.5GB
			EstimatedTime:   3 * time.Minute,
			EstimatedCost:   0.15,
			Confidence:      0.9,
		},
		StrategyHybridParallel: {
			EstimatedCPU:    2000,
			EstimatedMemory: 2 * 1024 * 1024 * 1024, // 2GB
			EstimatedTime:   90 * time.Second,
			EstimatedCost:   0.25,
			Confidence:      0.7,
		},
		StrategyTreeSitter: {
			EstimatedCPU:    800,
			EstimatedMemory: 768 * 1024 * 1024, // 768MB
			EstimatedTime:   45 * time.Second,
			EstimatedCost:   0.08,
			Confidence:      0.75,
		},
	}

	if prediction, exists := predictions[strategyType]; exists {
		return prediction
	}

	// Default prediction
	return ResourcePrediction{
		EstimatedCPU:    1000,
		EstimatedMemory: 1024 * 1024 * 1024,
		EstimatedTime:   2 * time.Minute,
		EstimatedCost:   0.10,
		Confidence:      0.5,
	}
}

func (s *DefaultStrategySelector) estimateExecutionTime(strategy TransformationStrategy, complexity ComplexityAnalysis) time.Duration {
	basePrediction := s.getBaseResourcePrediction(strategy.Primary)
	baseTime := basePrediction.EstimatedTime

	// Adjust based on complexity
	complexityFactor := 1.0 + complexity.OverallComplexity
	adjustedTime := time.Duration(float64(baseTime) * complexityFactor)

	return adjustedTime
}

func (s *DefaultStrategySelector) assessRisks(strategy TransformationStrategy, request StrategyRequest, complexity ComplexityAnalysis) StrategyRiskAssessment {
	var riskFactors []RiskFactor

	// Strategy-specific risks
	switch strategy.Primary {
	case StrategyLLMOnly:
		riskFactors = append(riskFactors, RiskFactor{
			Type:        "llm_hallucination",
			Severity:    0.6,
			Probability: 0.3,
			Description: "LLM may generate incorrect transformations",
			Mitigation:  "Use validation sandbox and human review",
		})
	case StrategyOpenRewriteOnly:
		riskFactors = append(riskFactors, RiskFactor{
			Type:        "incomplete_transformation",
			Severity:    0.4,
			Probability: 0.2,
			Description: "OpenRewrite may miss complex patterns",
			Mitigation:  "Combine with LLM enhancement",
		})
	}

	// Complexity-based risks
	if complexity.OverallComplexity > 0.8 {
		riskFactors = append(riskFactors, RiskFactor{
			Type:        "high_complexity",
			Severity:    0.8,
			Probability: 0.4,
			Description: "High complexity increases failure probability",
			Mitigation:  "Use staged approach with validation",
		})
	}

	// Calculate overall risk
	overallRisk := 0.0
	failureProbability := 0.0
	for _, factor := range riskFactors {
		overallRisk += factor.Severity * factor.Probability
		failureProbability += factor.Probability
	}

	if len(riskFactors) > 0 {
		overallRisk = overallRisk / float64(len(riskFactors))
		failureProbability = failureProbability / float64(len(riskFactors))
	}

	mitigationSteps := []string{
		"Execute in sandbox environment",
		"Validate transformations before applying",
		"Maintain rollback capability",
	}

	return StrategyRiskAssessment{
		OverallRisk:        overallRisk,
		RiskFactors:        riskFactors,
		MitigationSteps:    mitigationSteps,
		FailureProbability: failureProbability,
	}
}

func (s *DefaultStrategySelector) buildReasoning(primary scoredStrategy, request StrategyRequest, complexity ComplexityAnalysis) StrategyReasoning {
	primaryFactors := []string{
		fmt.Sprintf("Strategy weight: %.2f", primary.factors["base_weight"]),
		fmt.Sprintf("Complexity factor: %.2f", primary.factors["complexity_factor"]),
	}

	if request.TimeConstraints.PreferredSpeed != "" {
		primaryFactors = append(primaryFactors, fmt.Sprintf("Time preference: %s", request.TimeConstraints.PreferredSpeed))
	}

	explanation := fmt.Sprintf("Selected %s strategy with confidence %.2f based on complexity analysis and constraints",
		primary.strategy.Primary, primary.score)

	return StrategyReasoning{
		PrimaryFactors:    primaryFactors,
		ComplexityScore:   complexity.OverallComplexity,
		HistoricalData:    "Based on recent transformation patterns",
		ConstraintFactors: s.getConstraintFactors(request),
		Explanation:       explanation,
	}
}

func (s *DefaultStrategySelector) getConstraintFactors(request StrategyRequest) []string {
	var factors []string

	if request.ResourceConstraints.MaxMemory > 0 {
		factors = append(factors, fmt.Sprintf("Memory limit: %d MB", request.ResourceConstraints.MaxMemory/(1024*1024)))
	}

	if request.QualityRequirements.MinConfidence > 0 {
		factors = append(factors, fmt.Sprintf("Min confidence: %.2f", request.QualityRequirements.MinConfidence))
	}

	if request.TimeConstraints.MaxDuration > 0 {
		factors = append(factors, fmt.Sprintf("Max duration: %v", request.TimeConstraints.MaxDuration))
	}

	return factors
}

func (s *DefaultStrategySelector) selectAlternativeStrategy(tried map[StrategyType]bool) *TransformationStrategy {
	// Try strategies in order of preference
	alternatives := []StrategyType{
		StrategyHybridSequential,
		StrategyTreeSitter,
		StrategyLLMOnly,
		StrategyOpenRewriteOnly,
	}

	for _, alt := range alternatives {
		if !tried[alt] {
			return &TransformationStrategy{
				Primary:    alt,
				Confidence: s.strategyWeights[alt],
			}
		}
	}

	return nil // All strategies tried
}
