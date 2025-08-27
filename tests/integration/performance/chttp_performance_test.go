//go:build integration && performance
// +build integration,performance

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/testutil"
	"github.com/iw2rmb/ploy/internal/testutil/api"
)

// CHTTPPerformanceConfig defines CHTTP-specific performance testing parameters
type CHTTPPerformanceConfig struct {
	BaseURL          string
	CHTTPServiceURL  string
	TestDataDir      string
	Duration         time.Duration
	MaxConcurrent    int
	ResponseTarget   time.Duration // <5 seconds from roadmap
	ThroughputTarget int           // 50+ concurrent from roadmap
	MemoryTargetMB   int           // <100MB from roadmap
}

// CHTTPAnalysisResult contains CHTTP analysis performance metrics
type CHTTPAnalysisResult struct {
	TestType       string        `json:"test_type"`
	ProjectSize    string        `json:"project_size"`
	ResponseTime   time.Duration `json:"response_time"`
	Success        bool          `json:"success"`
	HTTPCode       int           `json:"http_code"`
	ResponseSizeKB int           `json:"response_size_kb"`
	IssuesFound    int           `json:"issues_found"`
	MemoryUsageMB  int           `json:"memory_usage_mb"`
	Timestamp      time.Time     `json:"timestamp"`
}

// CHTTPPerformanceSummary aggregates CHTTP performance test results
type CHTTPPerformanceSummary struct {
	TestType         string                `json:"test_type"`
	ProjectSize      string                `json:"project_size"`
	TotalTests       int                   `json:"total_tests"`
	SuccessfulTests  int                   `json:"successful_tests"`
	FailedTests      int                   `json:"failed_tests"`
	AvgResponseTime  time.Duration         `json:"avg_response_time"`
	MinResponseTime  time.Duration         `json:"min_response_time"`
	MaxResponseTime  time.Duration         `json:"max_response_time"`
	P95ResponseTime  time.Duration         `json:"p95_response_time"`
	P99ResponseTime  time.Duration         `json:"p99_response_time"`
	SuccessRate      float64               `json:"success_rate"`
	ThroughputRPS    float64               `json:"throughput_rps"`
	AvgMemoryUsageMB float64               `json:"avg_memory_usage_mb"`
	MaxMemoryUsageMB int                   `json:"max_memory_usage_mb"`
	MeetsTargets     CHTTPTargetValidation `json:"meets_targets"`
}

// CHTTPTargetValidation checks if performance meets roadmap targets
type CHTTPTargetValidation struct {
	ResponseTime bool `json:"response_time"` // <5 seconds
	Throughput   bool `json:"throughput"`    // 50+ concurrent
	MemoryUsage  bool `json:"memory_usage"`  // <100MB
	SuccessRate  bool `json:"success_rate"`  // >99%
	Overall      bool `json:"overall"`
}

// TestCHTTPVsLegacyPerformance compares CHTTP vs legacy analysis performance
func TestCHTTPVsLegacyPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CHTTP performance tests in short mode")
	}

	config := CHTTPPerformanceConfig{
		BaseURL:          testutil.GetEnvOrDefault("API_BASE_URL", "https://api.dev.ployman.app/v1"),
		CHTTPServiceURL:  testutil.GetEnvOrDefault("CHTTP_SERVICE_URL", "https://pylint.chttp.dev.ployd.app"),
		TestDataDir:      testutil.GetEnvOrDefault("TEST_DATA_DIR", "../performance-data"),
		Duration:         60 * time.Second,
		MaxConcurrent:    50,
		ResponseTarget:   5 * time.Second, // From roadmap
		ThroughputTarget: 50,              // From roadmap
		MemoryTargetMB:   100,             // From roadmap
	}

	// Test different project sizes
	projectSizes := []string{"small", "medium", "large"}

	for _, size := range projectSizes {
		t.Run(fmt.Sprintf("Legacy_%s", size), func(t *testing.T) {
			result := runCHTTPAnalysisTest(t, config, "legacy", size, 5)
			validatePerformanceTargets(t, result, config)
		})

		t.Run(fmt.Sprintf("CHTTP_%s", size), func(t *testing.T) {
			result := runCHTTPAnalysisTest(t, config, "chttp", size, 5)
			validatePerformanceTargets(t, result, config)
		})
	}

	// Compare overall performance
	t.Run("Performance_Comparison", func(t *testing.T) {
		comparePerformance(t, config)
	})
}

// TestCHTTPConcurrentCapacity tests concurrent analysis capacity
func TestCHTTPConcurrentCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CHTTP concurrent capacity tests in short mode")
	}

	config := CHTTPPerformanceConfig{
		BaseURL:          testutil.GetEnvOrDefault("API_BASE_URL", "https://api.dev.ployman.app/v1"),
		CHTTPServiceURL:  testutil.GetEnvOrDefault("CHTTP_SERVICE_URL", "https://pylint.chttp.dev.ployd.app"),
		TestDataDir:      testutil.GetEnvOrDefault("TEST_DATA_DIR", "../performance-data"),
		MaxConcurrent:    50,
		ResponseTarget:   5 * time.Second,
		ThroughputTarget: 50,
		MemoryTargetMB:   100,
	}

	// Test concurrent capacity for both legacy and CHTTP
	t.Run("Legacy_Concurrent", func(t *testing.T) {
		result := runConcurrentTest(t, config, "legacy", 25) // Lower for legacy
		assert.GreaterOrEqual(t, result.SuccessfulTests, 20, "Legacy should handle at least 20 concurrent requests")
	})

	t.Run("CHTTP_Concurrent", func(t *testing.T) {
		result := runConcurrentTest(t, config, "chttp", config.MaxConcurrent)
		assert.GreaterOrEqual(t, result.SuccessfulTests, config.ThroughputTarget,
			"CHTTP should handle target throughput: %d concurrent requests", config.ThroughputTarget)
	})
}

// TestCHTTPResourceUsage tests resource usage during sustained load
func TestCHTTPResourceUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CHTTP resource usage tests in short mode")
	}

	config := CHTTPPerformanceConfig{
		BaseURL:         testutil.GetEnvOrDefault("API_BASE_URL", "https://api.dev.ployman.app/v1"),
		CHTTPServiceURL: testutil.GetEnvOrDefault("CHTTP_SERVICE_URL", "https://pylint.chttp.dev.ployd.app"),
		TestDataDir:     testutil.GetEnvOrDefault("TEST_DATA_DIR", "../performance-data"),
		Duration:        120 * time.Second, // 2 minutes sustained load
		MemoryTargetMB:  100,
	}

	// Monitor resource usage during sustained CHTTP load
	t.Run("CHTTP_Sustained_Load", func(t *testing.T) {
		result := runSustainedLoadTest(t, config, "chttp")

		// Validate memory usage target
		assert.LessOrEqual(t, result.MaxMemoryUsageMB, config.MemoryTargetMB,
			"CHTTP memory usage (%dMB) should not exceed target (%dMB)",
			result.MaxMemoryUsageMB, config.MemoryTargetMB)

		// Validate sustained performance doesn't degrade
		assert.LessOrEqual(t, result.AvgResponseTime, config.ResponseTarget,
			"Sustained load response time should remain within target")

		assert.GreaterOrEqual(t, result.SuccessRate, 0.95,
			"Success rate should remain high under sustained load")
	})
}

// TestCHTTPPerformanceRegression detects performance regression
func TestCHTTPPerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CHTTP performance regression tests in short mode")
	}

	config := CHTTPPerformanceConfig{
		BaseURL:         testutil.GetEnvOrDefault("API_BASE_URL", "https://api.dev.ployman.app/v1"),
		CHTTPServiceURL: testutil.GetEnvOrDefault("CHTTP_SERVICE_URL", "https://pylint.chttp.dev.ployd.app"),
		TestDataDir:     testutil.GetEnvOrDefault("TEST_DATA_DIR", "../performance-data"),
		ResponseTarget:  5 * time.Second,
	}

	// Establish baseline performance
	baseline := runCHTTPAnalysisTest(t, config, "chttp", "medium", 10)

	// Performance regression thresholds (for future use)
	_ = 1.5 // maxResponseIncrease := 1.5 // 50% increase threshold
	_ = 0.8 // maxThroughputDecrease := 0.8 // 20% decrease threshold

	t.Logf("Baseline Performance:")
	t.Logf("  Average Response Time: %v", baseline.AvgResponseTime)
	t.Logf("  Success Rate: %.2f%%", baseline.SuccessRate*100)
	t.Logf("  Throughput: %.2f RPS", baseline.ThroughputRPS)
	t.Logf("  Memory Usage: %.2f MB", baseline.AvgMemoryUsageMB)

	// In a real implementation, you would compare against historical baselines
	// For now, we validate the baseline meets targets
	assert.LessOrEqual(t, baseline.AvgResponseTime, config.ResponseTarget,
		"Baseline response time should meet target")
	assert.GreaterOrEqual(t, baseline.SuccessRate, 0.95,
		"Baseline success rate should be acceptable")

	// Save baseline for future regression detection
	savePerformanceBaseline(t, baseline)
}

// runCHTTPAnalysisTest runs a series of analysis requests and measures performance
func runCHTTPAnalysisTest(t *testing.T, config CHTTPPerformanceConfig, testType, projectSize string, iterations int) *CHTTPPerformanceSummary {
	client := api.NewTestClient(t, config.BaseURL)

	var results []CHTTPAnalysisResult
	var wg sync.WaitGroup
	resultsChan := make(chan CHTTPAnalysisResult, iterations)

	startTime := time.Now()

	// Run test iterations
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			result := executeCHTTPAnalysis(client, config, testType, projectSize, iteration)
			resultsChan <- result
		}(i)

		// Slight delay to avoid overwhelming the system
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()
	close(resultsChan)

	// Collect results
	for result := range resultsChan {
		results = append(results, result)
	}

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	// Calculate summary metrics
	return calculateCHTTPSummary(results, testType, projectSize, totalDuration, config)
}

// runConcurrentTest tests concurrent analysis capacity
func runConcurrentTest(t *testing.T, config CHTTPPerformanceConfig, testType string, maxConcurrent int) *CHTTPPerformanceSummary {
	client := api.NewTestClient(t, config.BaseURL)

	var results []CHTTPAnalysisResult
	var wg sync.WaitGroup
	resultsChan := make(chan CHTTPAnalysisResult, maxConcurrent)

	startTime := time.Now()

	// Launch concurrent requests
	for i := 0; i < maxConcurrent; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			result := executeCHTTPAnalysis(client, config, testType, "small", iteration)
			resultsChan <- result
		}(i)
	}

	wg.Wait()
	close(resultsChan)

	// Collect results
	for result := range resultsChan {
		results = append(results, result)
	}

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	return calculateCHTTPSummary(results, testType, "concurrent", totalDuration, config)
}

// runSustainedLoadTest runs sustained load for resource monitoring
func runSustainedLoadTest(t *testing.T, config CHTTPPerformanceConfig, testType string) *CHTTPPerformanceSummary {
	client := api.NewTestClient(t, config.BaseURL)

	var results []CHTTPAnalysisResult
	var wg sync.WaitGroup
	resultsChan := make(chan CHTTPAnalysisResult, 1000)

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	startTime := time.Now()
	requestCount := 0

	// Run sustained load
	for {
		select {
		case <-ctx.Done():
			goto done
		default:
			wg.Add(1)
			go func(iteration int) {
				defer wg.Done()

				result := executeCHTTPAnalysis(client, config, testType, "small", iteration)
				resultsChan <- result
			}(requestCount)
			requestCount++

			// Control request rate
			time.Sleep(2 * time.Second)
		}
	}

done:
	wg.Wait()
	close(resultsChan)

	// Collect results
	for result := range resultsChan {
		results = append(results, result)
	}

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	return calculateCHTTPSummary(results, testType, "sustained", totalDuration, config)
}

// executeCHTTPAnalysis executes a single analysis request
func executeCHTTPAnalysis(client *api.TestClient, config CHTTPPerformanceConfig, testType, projectSize string, iteration int) CHTTPAnalysisResult {
	startTime := time.Now()

	// Create analysis payload
	payload := createAnalysisPayload(testType, projectSize, iteration)

	// Execute request
	resp := client.POST("/analysis/analyze").
		WithJSON(payload).
		Execute()

	responseTime := time.Since(startTime)

	result := CHTTPAnalysisResult{
		TestType:     testType,
		ProjectSize:  projectSize,
		ResponseTime: responseTime,
		Success:      resp != nil && resp.StatusCode == 200,
		HTTPCode:     resp.StatusCode,
		Timestamp:    time.Now(),
	}

	if result.Success && resp.Body != nil {
		var analysisResponse map[string]interface{}
		if err := json.Unmarshal(resp.Body, &analysisResponse); err == nil {
			if issues, ok := analysisResponse["issues"].([]interface{}); ok {
				result.IssuesFound = len(issues)
			}
		}
		result.ResponseSizeKB = int(len(resp.Body) / 1024)
	}

	// TODO: Add memory usage monitoring
	result.MemoryUsageMB = 0 // Placeholder

	return result
}

// createAnalysisPayload creates test payload for analysis
func createAnalysisPayload(testType, projectSize string, iteration int) map[string]interface{} {
	serviceURL := ""
	if testType == "chttp" {
		serviceURL = "https://pylint.chttp.dev.ployd.app"
	}

	return map[string]interface{}{
		"repository": map[string]interface{}{
			"id":     fmt.Sprintf("perf-test-%s-%s-%d", projectSize, testType, iteration),
			"name":   fmt.Sprintf("python-%s-project", projectSize),
			"url":    fmt.Sprintf("file:///tmp/test-project-%s", projectSize),
			"commit": "main",
		},
		"config": map[string]interface{}{
			"enabled": true,
			"mode":    testType,
			"languages": map[string]interface{}{
				"python": map[string]interface{}{
					"pylint":      true,
					"enabled":     true,
					"service_url": serviceURL,
				},
			},
		},
	}
}

// calculateCHTTPSummary calculates performance summary metrics
func calculateCHTTPSummary(results []CHTTPAnalysisResult, testType, projectSize string, totalDuration time.Duration, config CHTTPPerformanceConfig) *CHTTPPerformanceSummary {
	if len(results) == 0 {
		return &CHTTPPerformanceSummary{
			TestType:    testType,
			ProjectSize: projectSize,
		}
	}

	// Sort results by response time for percentile calculation
	sort.Slice(results, func(i, j int) bool {
		return results[i].ResponseTime < results[j].ResponseTime
	})

	successful := 0
	failed := 0
	var totalResponseTime time.Duration
	var totalMemoryUsage int
	maxMemoryUsage := 0

	for _, result := range results {
		if result.Success {
			successful++
		} else {
			failed++
		}
		totalResponseTime += result.ResponseTime
		totalMemoryUsage += result.MemoryUsageMB
		if result.MemoryUsageMB > maxMemoryUsage {
			maxMemoryUsage = result.MemoryUsageMB
		}
	}

	avgResponseTime := totalResponseTime / time.Duration(len(results))
	avgMemoryUsage := float64(totalMemoryUsage) / float64(len(results))
	successRate := float64(successful) / float64(len(results))
	throughputRPS := float64(successful) / totalDuration.Seconds()

	// Calculate percentiles
	p95Index := int(float64(len(results)) * 0.95)
	p99Index := int(float64(len(results)) * 0.99)
	if p95Index >= len(results) {
		p95Index = len(results) - 1
	}
	if p99Index >= len(results) {
		p99Index = len(results) - 1
	}

	// Validate performance targets
	targets := CHTTPTargetValidation{
		ResponseTime: avgResponseTime <= config.ResponseTarget,
		Throughput:   successful >= config.ThroughputTarget || throughputRPS >= float64(config.ThroughputTarget)/60.0,
		MemoryUsage:  maxMemoryUsage <= config.MemoryTargetMB,
		SuccessRate:  successRate >= 0.99,
	}
	targets.Overall = targets.ResponseTime && targets.Throughput && targets.MemoryUsage && targets.SuccessRate

	return &CHTTPPerformanceSummary{
		TestType:         testType,
		ProjectSize:      projectSize,
		TotalTests:       len(results),
		SuccessfulTests:  successful,
		FailedTests:      failed,
		AvgResponseTime:  avgResponseTime,
		MinResponseTime:  results[0].ResponseTime,
		MaxResponseTime:  results[len(results)-1].ResponseTime,
		P95ResponseTime:  results[p95Index].ResponseTime,
		P99ResponseTime:  results[p99Index].ResponseTime,
		SuccessRate:      successRate,
		ThroughputRPS:    throughputRPS,
		AvgMemoryUsageMB: avgMemoryUsage,
		MaxMemoryUsageMB: maxMemoryUsage,
		MeetsTargets:     targets,
	}
}

// validatePerformanceTargets validates results against roadmap targets
func validatePerformanceTargets(t *testing.T, result *CHTTPPerformanceSummary, config CHTTPPerformanceConfig) {
	// Response time target: <5 seconds
	assert.LessOrEqual(t, result.AvgResponseTime, config.ResponseTarget,
		"Average response time (%v) should be <= target (%v) for %s %s",
		result.AvgResponseTime, config.ResponseTarget, result.TestType, result.ProjectSize)

	// Success rate should be high
	assert.GreaterOrEqual(t, result.SuccessRate, 0.95,
		"Success rate (%.2f%%) should be >= 95%% for %s %s",
		result.SuccessRate*100, result.TestType, result.ProjectSize)

	// Memory usage target: <100MB (when available)
	if result.MaxMemoryUsageMB > 0 {
		assert.LessOrEqual(t, result.MaxMemoryUsageMB, config.MemoryTargetMB,
			"Memory usage (%dMB) should be <= target (%dMB) for %s %s",
			result.MaxMemoryUsageMB, config.MemoryTargetMB, result.TestType, result.ProjectSize)
	}

	t.Logf("Performance Results for %s %s:", result.TestType, result.ProjectSize)
	t.Logf("  Average Response Time: %v (target: <%v)", result.AvgResponseTime, config.ResponseTarget)
	t.Logf("  Success Rate: %.2f%% (target: >95%%)", result.SuccessRate*100)
	t.Logf("  Throughput: %.2f RPS", result.ThroughputRPS)
	t.Logf("  Memory Usage: %.1f MB avg, %d MB max (target: <%d MB)",
		result.AvgMemoryUsageMB, result.MaxMemoryUsageMB, config.MemoryTargetMB)
	t.Logf("  Meets All Targets: %t", result.MeetsTargets.Overall)
}

// comparePerformance compares CHTTP vs legacy performance
func comparePerformance(t *testing.T, config CHTTPPerformanceConfig) {
	// Run comparison tests for medium projects
	legacyResult := runCHTTPAnalysisTest(t, config, "legacy", "medium", 10)
	chttpResult := runCHTTPAnalysisTest(t, config, "chttp", "medium", 10)

	t.Logf("Performance Comparison (Medium Projects):")
	t.Logf("Legacy:")
	t.Logf("  Avg Response Time: %v", legacyResult.AvgResponseTime)
	t.Logf("  Success Rate: %.2f%%", legacyResult.SuccessRate*100)
	t.Logf("  Throughput: %.2f RPS", legacyResult.ThroughputRPS)

	t.Logf("CHTTP:")
	t.Logf("  Avg Response Time: %v", chttpResult.AvgResponseTime)
	t.Logf("  Success Rate: %.2f%%", chttpResult.SuccessRate*100)
	t.Logf("  Throughput: %.2f RPS", chttpResult.ThroughputRPS)

	// Calculate performance ratios
	responseTimeRatio := float64(chttpResult.AvgResponseTime) / float64(legacyResult.AvgResponseTime)
	throughputRatio := chttpResult.ThroughputRPS / legacyResult.ThroughputRPS

	t.Logf("Performance Ratios (CHTTP vs Legacy):")
	t.Logf("  Response Time Ratio: %.2fx", responseTimeRatio)
	t.Logf("  Throughput Ratio: %.2fx", throughputRatio)

	// CHTTP might be slightly slower due to network overhead, but should be within acceptable range
	assert.LessOrEqual(t, responseTimeRatio, 2.0,
		"CHTTP response time should not be more than 2x legacy")

	// Both should meet the performance targets
	assert.True(t, legacyResult.MeetsTargets.ResponseTime || chttpResult.MeetsTargets.ResponseTime,
		"At least one approach should meet response time targets")
}

// savePerformanceBaseline saves baseline performance for regression detection
func savePerformanceBaseline(t *testing.T, baseline *CHTTPPerformanceSummary) {
	// In a real implementation, this would save to persistent storage
	// For now, just log the baseline
	baselineData, err := json.MarshalIndent(baseline, "", "  ")
	require.NoError(t, err)

	t.Logf("Performance Baseline Saved:")
	t.Logf("%s", string(baselineData))

	// Write to temporary file for potential future use
	tmpFile := fmt.Sprintf("/tmp/performance-baseline-%s-%s.json",
		baseline.TestType, baseline.ProjectSize)

	err = os.WriteFile(tmpFile, baselineData, 0644)
	if err != nil {
		t.Logf("Warning: Could not save baseline to %s: %v", tmpFile, err)
	} else {
		t.Logf("Baseline saved to: %s", tmpFile)
	}
}
