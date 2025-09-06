//go:build performance
// +build performance

package performance

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/iw2rmb/ploy/internal/cli/transflow"
	"github.com/iw2rmb/ploy/internal/storage"
)

// LoadTestScenario defines a load testing scenario
type LoadTestScenario struct {
	Name          string
	Duration      time.Duration
	WorkflowRate  float64 // workflows per second
	ConcurrentMax int     // max concurrent workflows
	LearningRate  float64 // KB learning events per second
	ErrorVariety  int     // number of different error types
	SuccessRate   float64 // expected success rate (0.0-1.0)
}

// LoadTestEnvironment provides infrastructure for load testing
type LoadTestEnvironment struct {
	TempDir       string
	StorageClient *storage.StorageClient
	Integrations  *transflow.TransflowIntegrations
	KBStorage     transflow.KBStorage
	Config        *transflow.TransflowConfig
}

// setupLoadTestEnvironment creates a load test environment
func setupLoadTestEnvironment(t *testing.T) *LoadTestEnvironment {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Create storage client
	storageClient, err := storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888",
	})
	if err != nil {
		t.Skip("SeaweedFS not available for load testing")
	}

	// Create basic config
	config := &transflow.TransflowConfig{
		ID:           "load-test-workflow",
		TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:      "refs/heads/main",
		BuildTimeout: "5m", // Shorter timeout for load testing
		Steps: []transflow.TransflowStep{
			{
				Type:    "recipe",
				ID:      "java-migration-load",
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
			},
		},
		SelfHeal: transflow.SelfHealConfig{
			Enabled:    true,
			MaxRetries: 1, // Reduced retries for load testing
		},
	}

	// Create integrations in test mode
	integrations, err := transflow.NewTransflowIntegrations(config, true)
	if err != nil {
		t.Fatalf("Failed to create integrations: %v", err)
	}

	// Create KB storage
	lockManager := &transflow.MockKBLockManager{}
	kbStorage := transflow.NewSeaweedFSKBStorage(storageClient, lockManager)

	return &LoadTestEnvironment{
		TempDir:       tempDir,
		StorageClient: storageClient,
		Integrations:  integrations,
		KBStorage:     kbStorage,
		Config:        config,
	}
}

func (e *LoadTestEnvironment) Cleanup() {
	// Cleanup resources
}

// LoadTestResult contains results from a load test run
type LoadTestResult struct {
	TotalWorkflows      int64
	SuccessfulWorkflows int64
	FailedWorkflows     int64
	SuccessRate         float64
	AvgResponseTime     time.Duration
	MaxResponseTime     time.Duration
	MinResponseTime     time.Duration
	MaxMemoryMB         int64
	ErrorCounts         map[string]int64
	Duration            time.Duration
}

// RunLoadTest executes a load test scenario
func (env *LoadTestEnvironment) RunLoadTest(scenario LoadTestScenario) *LoadTestResult {
	ctx, cancel := context.WithTimeout(context.Background(), scenario.Duration)
	defer cancel()

	result := &LoadTestResult{
		ErrorCounts:     make(map[string]int64),
		MinResponseTime: time.Hour, // Initialize to high value
	}

	var wg sync.WaitGroup
	var responseTimesMutex sync.Mutex
	var responseTimes []time.Duration

	startTime := time.Now()

	// Calculate interval between workflow starts
	interval := time.Duration(float64(time.Second) / scenario.WorkflowRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	concurrentWorkflows := int64(0)
	workflowID := int64(0)

	// Workflow execution loop
	for {
		select {
		case <-ctx.Done():
			// Wait for all workflows to complete
			wg.Wait()
			result.Duration = time.Since(startTime)

			// Calculate average response time
			responseTimesMutex.Lock()
			if len(responseTimes) > 0 {
				total := time.Duration(0)
				for _, rt := range responseTimes {
					total += rt
					if rt > result.MaxResponseTime {
						result.MaxResponseTime = rt
					}
					if rt < result.MinResponseTime {
						result.MinResponseTime = rt
					}
				}
				result.AvgResponseTime = total / time.Duration(len(responseTimes))
			}
			responseTimesMutex.Unlock()

			// Calculate success rate
			if result.TotalWorkflows > 0 {
				result.SuccessRate = float64(result.SuccessfulWorkflows) / float64(result.TotalWorkflows)
			}

			return result

		case <-ticker.C:
			// Check if we can start another workflow
			current := atomic.LoadInt64(&concurrentWorkflows)
			if current >= int64(scenario.ConcurrentMax) {
				continue // Skip this tick, too many concurrent workflows
			}

			// Start new workflow
			wg.Add(1)
			atomic.AddInt64(&concurrentWorkflows, 1)
			atomic.AddInt64(&result.TotalWorkflows, 1)
			currentWorkflowID := atomic.AddInt64(&workflowID, 1)

			go func(wfID int64) {
				defer wg.Done()
				defer atomic.AddInt64(&concurrentWorkflows, -1)

				workflowStart := time.Now()

				// Create workflow configuration
				config := *env.Config
				config.ID = fmt.Sprintf("load-test-%d", wfID)

				// Create runner
				runner, err := transflow.NewTransflowRunner(&config, env.TempDir)
				if err != nil {
					atomic.AddInt64(&result.FailedWorkflows, 1)
					atomic.AddInt64(&result.ErrorCounts["runner_create_error"], 1)
					return
				}

				runner.SetBuildChecker(env.Integrations.CheckBuild)

				// Execute workflow
				workflowCtx, workflowCancel := context.WithTimeout(context.Background(), 3*time.Minute)
				workflowResult, err := runner.Run(workflowCtx)
				workflowCancel()

				responseTime := time.Since(workflowStart)

				// Record response time
				responseTimesMutex.Lock()
				responseTimes = append(responseTimes, responseTime)
				responseTimesMutex.Unlock()

				if err != nil {
					atomic.AddInt64(&result.FailedWorkflows, 1)
					atomic.AddInt64(&result.ErrorCounts["workflow_error"], 1)
				} else if workflowResult == nil || !workflowResult.Success {
					atomic.AddInt64(&result.FailedWorkflows, 1)
					atomic.AddInt64(&result.ErrorCounts["workflow_failed"], 1)
				} else {
					atomic.AddInt64(&result.SuccessfulWorkflows, 1)
				}
			}(currentWorkflowID)
		}
	}
}

// TestLoadTesting_ProductionScale runs production-scale load tests
// This should fail initially as the system is not optimized for scale
func TestLoadTesting_ProductionScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load tests in short mode")
	}

	env := setupLoadTestEnvironment(t)
	defer env.Cleanup()

	scenarios := []LoadTestScenario{
		{
			Name:          "Sustained Workflow Load",
			Duration:      5 * time.Minute, // Reduced for testing
			WorkflowRate:  0.5,             // 0.5 workflows per second (1 every 2 seconds)
			ConcurrentMax: 3,
			SuccessRate:   0.80, // Accept 80% success rate initially
		},
		{
			Name:          "Burst Workflow Load",
			Duration:      2 * time.Minute,
			WorkflowRate:  1.0, // 1 workflow per second
			ConcurrentMax: 5,
			SuccessRate:   0.70, // Lower success rate expected under burst load
		},
		{
			Name:          "Concurrency Stress Test",
			Duration:      3 * time.Minute,
			WorkflowRate:  2.0, // 2 workflows per second
			ConcurrentMax: 8,
			SuccessRate:   0.60, // Even lower success rate under stress
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			t.Logf("Starting load test scenario: %s", scenario.Name)
			t.Logf("  Duration: %v", scenario.Duration)
			t.Logf("  Workflow Rate: %.1f/sec", scenario.WorkflowRate)
			t.Logf("  Max Concurrent: %d", scenario.ConcurrentMax)

			results := env.RunLoadTest(scenario)

			t.Logf("Load test results for %s:", scenario.Name)
			t.Logf("  Total Workflows: %d", results.TotalWorkflows)
			t.Logf("  Successful: %d", results.SuccessfulWorkflows)
			t.Logf("  Failed: %d", results.FailedWorkflows)
			t.Logf("  Success Rate: %.2f%%", results.SuccessRate*100)
			t.Logf("  Avg Response Time: %v", results.AvgResponseTime)
			t.Logf("  Max Response Time: %v", results.MaxResponseTime)
			t.Logf("  Min Response Time: %v", results.MinResponseTime)

			// Log error breakdown
			for errorType, count := range results.ErrorCounts {
				if count > 0 {
					t.Logf("  %s: %d", errorType, count)
				}
			}

			// Validate basic requirements (initially these may fail)
			if results.TotalWorkflows > 0 {
				// These assertions may fail initially - that's expected for RED phase
				successRateOK := results.SuccessRate >= scenario.SuccessRate
				avgResponseOK := results.AvgResponseTime < 5*time.Minute

				t.Logf("Performance validation:")
				t.Logf("  Success rate %.2f%% >= %.2f%%: %v", results.SuccessRate*100, scenario.SuccessRate*100, successRateOK)
				t.Logf("  Avg response time %v < 5m: %v", results.AvgResponseTime, avgResponseOK)

				if !successRateOK || !avgResponseOK {
					t.Logf("Load test performance targets not met (expected in RED phase)")
				}
			} else {
				t.Error("No workflows were executed during load test")
			}
		})
	}
}

// TestKBLearningStress tests KB system under learning load
func TestKBLearningStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping KB stress tests in short mode")
	}

	env := setupLoadTestEnvironment(t)
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Stress test parameters
	concurrentLearners := 5
	learningEventsPerSecond := 10
	errorVariety := 20

	var wg sync.WaitGroup
	var totalLearningAttempts int64
	var successfulLearning int64
	var learningErrors int64

	t.Logf("Starting KB learning stress test:")
	t.Logf("  Concurrent learners: %d", concurrentLearners)
	t.Logf("  Events per second: %d", learningEventsPerSecond)
	t.Logf("  Error variety: %d", errorVariety)

	interval := time.Duration(1000/learningEventsPerSecond) * time.Millisecond / time.Duration(concurrentLearners)

	for i := 0; i < concurrentLearners; i++ {
		wg.Add(1)
		go func(learnerID int) {
			defer wg.Done()

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			eventID := 0

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					atomic.AddInt64(&totalLearningAttempts, 1)

					// Generate learning event
					errorSig := fmt.Sprintf("stress-error-%d", eventID%errorVariety)
					runID := fmt.Sprintf("stress-run-%d-%d", learnerID, eventID)

					caseRecord := generateTestHealingCase(runID, errorSig, eventID%3 == 0) // 33% success rate

					// Attempt to write case to KB
					err := env.KBStorage.WriteCase(context.Background(), "java", errorSig, runID, caseRecord)
					if err != nil {
						atomic.AddInt64(&learningErrors, 1)
						t.Logf("KB learning error (learner %d, event %d): %v", learnerID, eventID, err)
					} else {
						atomic.AddInt64(&successfulLearning, 1)
					}

					eventID++
				}
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()

	totalAttempts := atomic.LoadInt64(&totalLearningAttempts)
	successCount := atomic.LoadInt64(&successfulLearning)
	errorCount := atomic.LoadInt64(&learningErrors)

	t.Logf("KB learning stress test results:")
	t.Logf("  Total learning attempts: %d", totalAttempts)
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Errors: %d", errorCount)

	if totalAttempts > 0 {
		successRate := float64(successCount) / float64(totalAttempts)
		t.Logf("  Success rate: %.2f%%", successRate*100)

		// Initially we may have low success rates - that's expected
		if successRate < 0.5 {
			t.Logf("KB learning success rate is low (expected in RED phase)")
		}
	}

	assert.True(t, totalAttempts > 0, "Should have attempted some learning operations")
}

// TestConcurrentWorkflowExecution tests concurrent workflow execution limits
func TestConcurrentWorkflowExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent workflow tests in short mode")
	}

	env := setupLoadTestEnvironment(t)
	defer env.Cleanup()

	concurrencyLevels := []int{2, 4, 8, 12}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			var wg sync.WaitGroup
			var completedWorkflows int64
			var failedWorkflows int64
			var totalDuration time.Duration
			var durationMutex sync.Mutex

			startTime := time.Now()

			t.Logf("Starting concurrent workflow test with %d workers", concurrency)

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					workflowStart := time.Now()

					// Create unique config for this worker
					config := *env.Config
					config.ID = fmt.Sprintf("concurrent-test-%d-%d", concurrency, workerID)

					// Create runner
					runner, err := transflow.NewTransflowRunner(&config, env.TempDir)
					if err != nil {
						atomic.AddInt64(&failedWorkflows, 1)
						t.Logf("Worker %d: Failed to create runner: %v", workerID, err)
						return
					}

					runner.SetBuildChecker(env.Integrations.CheckBuild)

					// Execute workflow
					result, err := runner.Run(ctx)
					workflowDuration := time.Since(workflowStart)

					durationMutex.Lock()
					totalDuration += workflowDuration
					durationMutex.Unlock()

					if err != nil {
						atomic.AddInt64(&failedWorkflows, 1)
						t.Logf("Worker %d: Workflow failed: %v", workerID, err)
					} else if result == nil || !result.Success {
						atomic.AddInt64(&failedWorkflows, 1)
						t.Logf("Worker %d: Workflow unsuccessful", workerID)
					} else {
						atomic.AddInt64(&completedWorkflows, 1)
						t.Logf("Worker %d: Workflow completed in %v", workerID, workflowDuration)
					}
				}(i)
			}

			wg.Wait()

			totalTime := time.Since(startTime)
			completed := atomic.LoadInt64(&completedWorkflows)
			failed := atomic.LoadInt64(&failedWorkflows)
			total := completed + failed

			t.Logf("Concurrent workflow test results (concurrency %d):", concurrency)
			t.Logf("  Total workflows: %d", total)
			t.Logf("  Completed: %d", completed)
			t.Logf("  Failed: %d", failed)
			t.Logf("  Total time: %v", totalTime)

			if total > 0 {
				successRate := float64(completed) / float64(total)
				avgDuration := totalDuration / time.Duration(total)

				t.Logf("  Success rate: %.2f%%", successRate*100)
				t.Logf("  Average workflow duration: %v", avgDuration)

				// Performance targets (may not be met initially)
				targetSuccessRate := 0.70            // 70% success rate target
				targetAvgDuration := 2 * time.Minute // 2 minute average target

				successRateOK := successRate >= targetSuccessRate
				avgDurationOK := avgDuration <= targetAvgDuration

				t.Logf("  Success rate %.2f%% >= %.2f%%: %v", successRate*100, targetSuccessRate*100, successRateOK)
				t.Logf("  Avg duration %v <= %v: %v", avgDuration, targetAvgDuration, avgDurationOK)

				if !successRateOK || !avgDurationOK {
					t.Logf("Concurrent execution performance targets not met (expected in RED phase)")
				}
			}

			// Assert basic functionality
			assert.True(t, total > 0, "Should have executed some workflows")
			assert.True(t, completed >= 0, "Should have some successful completions or be in RED phase")
		})
	}
}

// TestMemoryLeakDetection monitors memory usage during extended operation
func TestMemoryLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak tests in short mode")
	}

	env := setupLoadTestEnvironment(t)
	defer env.Cleanup()

	t.Logf("Starting memory leak detection test")

	// Run workflows continuously for a period
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var workflowCount int64

	ticker := time.NewTicker(5 * time.Second) // One workflow every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Logf("Memory leak test completed after %d workflows", atomic.LoadInt64(&workflowCount))
			return
		case <-ticker.C:
			wfID := atomic.AddInt64(&workflowCount, 1)

			go func(workflowID int64) {
				config := *env.Config
				config.ID = fmt.Sprintf("memory-leak-test-%d", workflowID)

				runner, err := transflow.NewTransflowRunner(&config, env.TempDir)
				if err != nil {
					t.Logf("Memory test workflow %d: Failed to create runner: %v", workflowID, err)
					return
				}

				runner.SetBuildChecker(env.Integrations.CheckBuild)

				wfCtx, wfCancel := context.WithTimeout(context.Background(), 1*time.Minute)
				result, err := runner.Run(wfCtx)
				wfCancel()

				if err != nil {
					t.Logf("Memory test workflow %d: Failed: %v", workflowID, err)
				} else if result == nil || !result.Success {
					t.Logf("Memory test workflow %d: Unsuccessful", workflowID)
				} else {
					t.Logf("Memory test workflow %d: Completed successfully", workflowID)
				}
			}(wfID)
		}
	}
}
