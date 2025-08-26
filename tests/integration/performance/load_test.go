//go:build integration
// +build integration

package performance

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/iw2rmb/ploy/internal/testutil"
	"github.com/iw2rmb/ploy/internal/testutil/api"
)

// LoadTestConfig defines load testing parameters
type LoadTestConfig struct {
	Duration    time.Duration
	Concurrency int
	RequestRate int // requests per second
	RampUpTime  time.Duration
}

// LoadTestResult contains load test metrics
type LoadTestResult struct {
	TotalRequests  int
	SuccessfulReqs int
	FailedRequests int
	AverageLatency time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration
	ErrorRate      float64
}

// TestAPILoadPerformance tests API performance under load
func TestAPILoadPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load tests in short mode")
	}

	config := LoadTestConfig{
		Duration:    30 * time.Second,
		Concurrency: 10,
		RequestRate: 50, // 50 RPS
		RampUpTime:  5 * time.Second,
	}

	result := runLoadTest(t, config)

	// Assert performance requirements
	assert.Less(t, result.ErrorRate, 0.01, "Error rate should be less than 1%")
	assert.Less(t, result.AverageLatency, 200*time.Millisecond,
		"Average latency should be less than 200ms")
	assert.Less(t, result.P95Latency, 500*time.Millisecond,
		"P95 latency should be less than 500ms")
	assert.Less(t, result.P99Latency, 1*time.Second,
		"P99 latency should be less than 1s")
}

// runLoadTest executes a load test with the given configuration
func runLoadTest(t *testing.T, config LoadTestConfig) *LoadTestResult {
	controllerURL := testutil.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")
	client := api.NewTestClient(t, controllerURL)

	// Channels for coordination
	start := make(chan struct{})
	results := make(chan requestResult, config.Concurrency*1000)
	done := make(chan struct{})

	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loadTestWorker(client, start, results, config)
		}()
	}

	// Collect results
	go func() {
		wg.Wait()
		close(results)
		done <- struct{}{}
	}()

	// Start load test
	close(start)

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed normally
	case <-time.After(config.Duration + 10*time.Second):
		t.Fatal("Load test timed out")
	}

	// Calculate metrics
	return calculateLoadTestMetrics(results)
}

type requestResult struct {
	success bool
	latency time.Duration
	error   error
}

// loadTestWorker performs requests for the duration of the test
func loadTestWorker(client *api.TestClient, start <-chan struct{},
	results chan<- requestResult, config LoadTestConfig) {

	<-start // Wait for test to start

	endTime := time.Now().Add(config.Duration)
	requestInterval := time.Duration(int64(time.Second) / int64(config.RequestRate/config.Concurrency))

	ticker := time.NewTicker(requestInterval)
	defer ticker.Stop()

	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			startTime := time.Now()

			// Make request to health endpoint (lightweight)
			resp := client.GET("/health").Execute()

			latency := time.Since(startTime)

			results <- requestResult{
				success: resp != nil && resp.StatusCode == 200,
				latency: latency,
				error:   nil,
			}

		case <-time.After(requestInterval * 2):
			// Timeout protection
			return
		}
	}
}

// calculateLoadTestMetrics computes performance metrics from results
func calculateLoadTestMetrics(results <-chan requestResult) *LoadTestResult {
	var (
		total      = 0
		successful = 0
		failed     = 0
		latencies  []time.Duration
	)

	// Collect all results
	for result := range results {
		total++
		latencies = append(latencies, result.latency)

		if result.success {
			successful++
		} else {
			failed++
		}
	}

	if total == 0 {
		return &LoadTestResult{}
	}

	// Sort latencies for percentile calculation
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	// Calculate average latency
	var totalLatency time.Duration
	for _, latency := range latencies {
		totalLatency += latency
	}
	avgLatency := totalLatency / time.Duration(len(latencies))

	// Calculate percentiles
	p95Index := int(float64(len(latencies)) * 0.95)
	p99Index := int(float64(len(latencies)) * 0.99)

	var p95Latency, p99Latency time.Duration
	if len(latencies) > 0 {
		if p95Index >= len(latencies) {
			p95Index = len(latencies) - 1
		}
		if p99Index >= len(latencies) {
			p99Index = len(latencies) - 1
		}

		p95Latency = latencies[p95Index]
		p99Latency = latencies[p99Index]
	}

	errorRate := float64(failed) / float64(total)

	return &LoadTestResult{
		TotalRequests:  total,
		SuccessfulReqs: successful,
		FailedRequests: failed,
		AverageLatency: avgLatency,
		P95Latency:     p95Latency,
		P99Latency:     p99Latency,
		ErrorRate:      errorRate,
	}
}

// TestConcurrentBuildRequests tests build endpoint under concurrent load
func TestConcurrentBuildRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent build tests in short mode")
	}

	controllerURL := testutil.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")
	client := api.NewTestClient(t, controllerURL)
	numConcurrent := 5

	var wg sync.WaitGroup
	results := make(chan error, numConcurrent)

	buildRequest := map[string]interface{}{
		"git_url": "https://github.com/test/go-app.git",
		"branch":  "main",
	}

	// Start concurrent build requests
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			appName := fmt.Sprintf("load-test-app-%d", id)

			resp := client.POST("/v1/apps/"+appName+"/builds").
				WithJSON(buildRequest).
				Execute()

			var err error
			if resp == nil || (resp.StatusCode != 202 && resp.StatusCode != 400) {
				err = fmt.Errorf("unexpected response status: %d", resp.StatusCode)
			}

			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// Check results
	errorCount := 0
	for err := range results {
		if err != nil {
			t.Logf("Concurrent request error: %v", err)
			errorCount++
		}
	}

	// Allow some errors but not too many
	assert.LessOrEqual(t, errorCount, numConcurrent/2,
		"More than half of concurrent requests failed")
}

// TestLatencyAndThroughputBenchmarks tests various performance scenarios
func TestLatencyAndThroughputBenchmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmarks in short mode")
	}

	scenarios := []struct {
		name        string
		concurrency int
		requestRate int
		duration    time.Duration
	}{
		{"Low Load", 1, 10, 10 * time.Second},
		{"Medium Load", 5, 25, 15 * time.Second},
		{"High Load", 10, 50, 20 * time.Second},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			config := LoadTestConfig{
				Duration:    scenario.duration,
				Concurrency: scenario.concurrency,
				RequestRate: scenario.requestRate,
				RampUpTime:  2 * time.Second,
			}

			result := runLoadTest(t, config)

			// Log performance metrics
			t.Logf("Scenario: %s", scenario.name)
			t.Logf("Total Requests: %d", result.TotalRequests)
			t.Logf("Successful: %d", result.SuccessfulReqs)
			t.Logf("Failed: %d", result.FailedRequests)
			t.Logf("Error Rate: %.2f%%", result.ErrorRate*100)
			t.Logf("Average Latency: %v", result.AverageLatency)
			t.Logf("P95 Latency: %v", result.P95Latency)
			t.Logf("P99 Latency: %v", result.P99Latency)

			// Basic assertions - adjust based on system capabilities
			assert.Less(t, result.ErrorRate, 0.05,
				"Error rate should be less than 5% for %s", scenario.name)
			assert.Greater(t, result.TotalRequests, scenario.requestRate*int(scenario.duration.Seconds())/2,
				"Should achieve reasonable throughput for %s", scenario.name)
		})
	}
}

// TestPerformanceRegression tests for performance regression detection
func TestPerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance regression tests in short mode")
	}

	// Baseline performance test
	baselineConfig := LoadTestConfig{
		Duration:    15 * time.Second,
		Concurrency: 5,
		RequestRate: 25,
		RampUpTime:  3 * time.Second,
	}

	baseline := runLoadTest(t, baselineConfig)

	// For regression testing, you would compare against historical baselines
	// Here we're just establishing the framework
	// Regression thresholds would be:
	// maxAllowedLatencyIncrease := 1.5 // 50% increase
	// maxAllowedErrorRateIncrease := 0.02 // 2% absolute increase
	t.Logf("Baseline Performance Metrics:")
	t.Logf("  Average Latency: %v", baseline.AverageLatency)
	t.Logf("  P95 Latency: %v", baseline.P95Latency)
	t.Logf("  Error Rate: %.2f%%", baseline.ErrorRate*100)
	t.Logf("  Total Requests: %d", baseline.TotalRequests)

	// Basic health checks
	assert.Less(t, baseline.ErrorRate, 0.05, "Baseline error rate should be reasonable")
	assert.Less(t, baseline.AverageLatency, 300*time.Millisecond,
		"Baseline latency should be reasonable")

	// This is where you would implement comparison with historical data
	// For example, load from persistent storage and compare:
	//
	// historicalBaseline := loadHistoricalBaseline()
	// latencyRatio := float64(baseline.AverageLatency) / float64(historicalBaseline.AverageLatency)
	// assert.Less(t, latencyRatio, maxAllowedLatencyIncrease, "Performance regression detected")

	t.Logf("Performance regression detection framework established")
}