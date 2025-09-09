package arf

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// analyzeTransformation provides deep analysis of a transformation and its healing attempts
func analyzeTransformation(status *TransformationStatus) *TransformationAnalysis {
	analysis := &TransformationAnalysis{
		TransformationID: status.TransformationID,
	}

	// Build summary
	totalAttempts := countAttempts(status.Children)
	successCount := countSuccessful(status.Children)

	var totalDuration time.Duration
	if !status.EndTime.IsZero() {
		totalDuration = status.EndTime.Sub(status.StartTime)
	} else {
		totalDuration = time.Since(status.StartTime)
	}

	// Calculate success rate with zero-attempt handling
	var successRate float64
	if totalAttempts > 0 {
		successRate = float64(successCount) / float64(totalAttempts)
	} else if status.Status == "completed" {
		// No healing attempts but transformation completed = 100% success
		successRate = 1.0
	}

	// Calculate max depth
	maxDepth := calculateMaxDepth(status.Children, 0)

	// Find most common error
	mostCommonError := ""
	if len(status.Children) > 0 {
		errorCounts := make(map[string]int)
		for _, attempt := range status.Children {
			if attempt.TriggerReason != "" {
				errorCounts[attempt.TriggerReason]++
			}
		}
		maxCount := 0
		for errorType, count := range errorCounts {
			if count > maxCount {
				maxCount = count
				mostCommonError = errorType
			}
		}
	}

	analysis.Summary = AnalysisSummary{
		TransformationID:   status.TransformationID,
		Status:             status.Status,
		TotalDurationHours: math.Round(totalDuration.Hours()*10) / 10, // Round to 1 decimal place
		TotalAttempts:      totalAttempts,
		SuccessRate:        successRate,
		MaxDepthReached:    maxDepth,
		MostCommonError:    mostCommonError,
		FinalResult:        status.Status,
	}

	// Analyze error patterns
	analysis.ErrorPatterns = analyzeErrorPatterns(status.Children)
	if analysis.ErrorPatterns == nil {
		analysis.ErrorPatterns = []HealingErrorPattern{}
	}

	// Enhanced cost analysis with zero-division protection
	totalCost := estimateCost(status.Children)
	infraCost := estimateInfrastructureCost(totalDuration)

	analysis.CostAnalysis = CostBreakdown{
		TotalCost:          totalCost + infraCost,
		LLMCost:            estimateLLMCost(status.Children),
		InfrastructureCost: infraCost,
		CostPerAttempt:     0.0, // Initialize to zero
		CostPerSuccess:     0.0, // Initialize to zero
		ROI:                calculateROI(totalCost+infraCost, totalDuration),
	}

	// Safe division with zero checks
	if totalAttempts > 0 {
		analysis.CostAnalysis.CostPerAttempt = analysis.CostAnalysis.TotalCost / float64(totalAttempts)
	}
	if successCount > 0 {
		analysis.CostAnalysis.CostPerSuccess = analysis.CostAnalysis.TotalCost / float64(successCount)
	} else if status.Status == "completed" && totalAttempts == 0 {
		// Successful transformation without healing attempts
		analysis.CostAnalysis.CostPerSuccess = analysis.CostAnalysis.TotalCost
	}

	// Performance metrics
	analysis.PerformanceMetrics = analyzePerformance(status.Children)

	// Add time to first success for completed transformations
	if status.Status == "completed" {
		analysis.PerformanceMetrics.TimeToFirstSuccess = totalDuration
	}

	// Generate recommendations
	analysis.Recommendations = generateRecommendations(analysis)

	// Add success factors for well-performing transformations
	analysis.SuccessFactors = identifySuccessFactors(status, totalAttempts, successRate)

	return analysis
}

// countAttempts recursively counts all healing attempts in the tree
func countAttempts(attempts []HealingAttempt) int {
	count := len(attempts)
	for _, attempt := range attempts {
		count += countAttempts(attempt.Children)
	}
	return count
}

// countSuccessful recursively counts successful healing attempts
func countSuccessful(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Result == "success" {
			count++
		}
		count += countSuccessful(attempt.Children)
	}
	return count
}

// estimateCost calculates estimated cost for healing attempts
func estimateCost(attempts []HealingAttempt) float64 {
	// Simplified cost estimation - $0.10 per attempt + LLM costs
	baseCost := float64(countAttempts(attempts)) * 0.10

	// Add estimated LLM costs
	llmCost := estimateLLMCost(attempts)

	return baseCost + llmCost
}

// estimateLLMCost calculates estimated cost for LLM API calls
func estimateLLMCost(attempts []HealingAttempt) float64 {
	llmCalls := 0
	var estimateLLMCalls func([]HealingAttempt)
	estimateLLMCalls = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.LLMAnalysis != nil {
				llmCalls++
			}
			// Recurse into children
			estimateLLMCalls(attempt.Children)
		}
	}

	estimateLLMCalls(attempts)
	return float64(llmCalls) * 0.05 // $0.05 per LLM call estimate
}

// estimateInfrastructureCost calculates infrastructure cost based on duration
func estimateInfrastructureCost(duration time.Duration) float64 {
	// Estimate based on compute time: $0.10 per hour for container execution
	hours := duration.Hours()
	if hours < 0.1 {
		hours = 0.1 // Minimum billing
	}
	return hours * 0.10
}

// calculateROI calculates return on investment for the transformation
func calculateROI(totalCost float64, duration time.Duration) float64 {
	// Estimate ROI based on developer time saved
	// Assume manual migration would take 4 hours at $50/hour = $200
	manualCost := 200.0
	if totalCost > 0 {
		return ((manualCost - totalCost) / totalCost) * 100
	}
	return 0.0
}

// analyzeErrorPatterns identifies common error patterns in healing attempts
func analyzeErrorPatterns(attempts []HealingAttempt) []HealingErrorPattern {
	errorCounts := make(map[string]*HealingErrorPattern)

	var analyze func([]HealingAttempt)
	analyze = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.TriggerReason != "" {
				pattern, exists := errorCounts[attempt.TriggerReason]
				if !exists {
					pattern = &HealingErrorPattern{
						ErrorType:    attempt.TriggerReason,
						AttemptPaths: []string{},
					}
					errorCounts[attempt.TriggerReason] = pattern
				}
				pattern.Frequency++
				pattern.AttemptPaths = append(pattern.AttemptPaths, attempt.AttemptPath)

				if attempt.Result == "success" {
					pattern.SuccessRate = (pattern.SuccessRate*float64(pattern.Frequency-1) + 1) / float64(pattern.Frequency)
				} else {
					pattern.SuccessRate = (pattern.SuccessRate * float64(pattern.Frequency-1)) / float64(pattern.Frequency)
				}
			}
			analyze(attempt.Children)
		}
	}

	analyze(attempts)

	// Convert map to slice
	var patterns []HealingErrorPattern
	for _, pattern := range errorCounts {
		patterns = append(patterns, *pattern)
	}

	// Sort by frequency
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Frequency > patterns[j].Frequency
	})

	return patterns
}

// analyzePerformance calculates performance metrics for healing attempts
func analyzePerformance(attempts []HealingAttempt) PerformanceAnalysis {
	var durations []time.Duration

	var collect func([]HealingAttempt)
	collect = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if !attempt.EndTime.IsZero() {
				duration := attempt.EndTime.Sub(attempt.StartTime)
				durations = append(durations, duration)
			}
			collect(attempt.Children)
		}
	}

	collect(attempts)

	perf := PerformanceAnalysis{
		DepthPerformance: make(map[int]time.Duration),
	}

	if len(durations) > 0 {
		// Calculate average
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		perf.AverageAttemptDuration = total / time.Duration(len(durations))

		// Calculate median
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		perf.MedianAttemptDuration = durations[len(durations)/2]

		// Calculate P95
		p95Index := int(float64(len(durations)) * 0.95)
		if p95Index < len(durations) {
			perf.P95AttemptDuration = durations[p95Index]
		}
	}

	return perf
}

// generateRecommendations generates actionable recommendations based on analysis
func generateRecommendations(analysis *TransformationAnalysis) []string {
	var recommendations []string

	// Handle successful transformations with no healing attempts
	if analysis.Summary.TotalAttempts == 0 && analysis.Summary.Status == "completed" {
		recommendations = append(recommendations, "Transformation performing within normal parameters.")
		recommendations = append(recommendations, "Excellent success rate with no healing required.")

		if analysis.Summary.TotalDurationHours < 0.5 {
			recommendations = append(recommendations, "Fast execution time - consider using this pattern as a template.")
		}

		if analysis.CostAnalysis.ROI > 50 {
			recommendations = append(recommendations, "High ROI transformation - significant cost savings achieved.")
		}

		return recommendations
	}

	// Check success rate for transformations with healing attempts
	if analysis.Summary.SuccessRate < 0.5 {
		recommendations = append(recommendations,
			"Low success rate detected. Consider reviewing healing strategies and error patterns.")
	} else if analysis.Summary.SuccessRate >= 0.8 {
		recommendations = append(recommendations,
			"High success rate achieved - healing strategies are working effectively.")
	}

	// Check for recurring errors
	if len(analysis.ErrorPatterns) > 0 && analysis.ErrorPatterns[0].Frequency > 3 {
		recommendations = append(recommendations,
			fmt.Sprintf("Frequent error pattern '%s' detected. Consider creating specific healing recipe.",
				analysis.ErrorPatterns[0].ErrorType))
	}

	// Check cost efficiency
	if analysis.CostAnalysis.CostPerSuccess > 10.0 {
		recommendations = append(recommendations,
			"High cost per successful heal. Consider optimizing LLM usage or healing strategies.")
	} else if analysis.CostAnalysis.CostPerSuccess < 1.0 {
		recommendations = append(recommendations,
			"Very cost-effective transformation. Current approach is optimal.")
	}

	// Check performance
	if analysis.PerformanceMetrics.P95AttemptDuration > 30*time.Minute {
		recommendations = append(recommendations,
			"Long-running healing attempts detected. Consider adding timeouts or parallel processing.")
	}

	// Check transformation duration
	if analysis.Summary.TotalDurationHours > 2.0 {
		recommendations = append(recommendations,
			"Long transformation duration. Consider breaking into smaller chunks or optimizing recipe selection.")
	}

	// Check healing depth
	if analysis.Summary.MaxDepthReached > 5 {
		recommendations = append(recommendations,
			"Deep healing tree detected. Consider reviewing initial recipe selection to reduce complexity.")
	}

	// ROI recommendations
	if analysis.CostAnalysis.ROI > 100 {
		recommendations = append(recommendations,
			"Excellent ROI achieved. This transformation approach is highly cost-effective.")
	} else if analysis.CostAnalysis.ROI < 0 {
		recommendations = append(recommendations,
			"Negative ROI detected. Consider optimizing costs or reviewing transformation necessity.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Transformation performing within normal parameters.")
	}

	return recommendations
}

// identifySuccessFactors identifies factors that contributed to successful transformations
func identifySuccessFactors(status *TransformationStatus, totalAttempts int, successRate float64) []string {
	var factors []string

	// Check for immediate success
	if totalAttempts == 0 && status.Status == "completed" {
		factors = append(factors, "Clean transformation requiring no healing attempts")
		factors = append(factors, "Well-structured codebase with compatible patterns")
		factors = append(factors, "Appropriate recipe selection for target transformation")
	}

	// Check for high success rate
	if successRate >= 0.8 {
		factors = append(factors, "High healing success rate indicates robust error recovery")
	}

	// Check for fast execution
	var duration time.Duration
	if !status.EndTime.IsZero() {
		duration = status.EndTime.Sub(status.StartTime)
	} else {
		duration = time.Since(status.StartTime)
	}

	if duration < 1*time.Minute {
		factors = append(factors, "Fast execution time indicates efficient processing")
	} else if duration < 5*time.Minute {
		factors = append(factors, "Reasonable execution time for transformation complexity")
	}

	return factors
}
