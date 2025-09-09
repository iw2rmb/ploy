//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadMultipleConcurrentTransformations tests system under concurrent load
func TestLoadMultipleConcurrentTransformations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Configuration
	const (
		numTransformations = 50
		numWorkers         = 10
		testDuration       = 30 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Setup
	mockConsul := NewMockConsulStore()
	config := DefaultHealingConfig()
	config.MaxParallelAttempts = 5
	config.QueueSize = 200

	coordinator := NewHealingCoordinator(config)
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Metrics tracking
	var (
		successCount     int64
		failureCount     int64
		totalDuration    int64
		completedCount   int64
		activeGoroutines int32
	)

	// Worker pool
	var wg sync.WaitGroup
	transformChan := make(chan string, numTransformations)

	// Generate transformation IDs
	for i := 0; i < numTransformations; i++ {
		transformChan <- uuid.New().String()
	}
	close(transformChan)

	// Start workers
	startTime := time.Now()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			atomic.AddInt32(&activeGoroutines, 1)
			defer atomic.AddInt32(&activeGoroutines, -1)

			for transformID := range transformChan {
				start := time.Now()

				// Create transformation with failure
				status := &TransformationStatus{
					TransformationID: transformID,
					Status:           "failed",
					WorkflowStage:    "build",
					StartTime:        time.Now(),
					Error:            fmt.Sprintf("Build error %d", workerID),
				}

				// Store in Consul
				if err := mockConsul.StoreTransformationStatus(ctx, transformID, status); err != nil {
					atomic.AddInt64(&failureCount, 1)
					continue
				}

				// Simulate healing attempts
				for i := 1; i <= 3; i++ {
					attempt := &HealingAttempt{
						TransformationID: uuid.New().String(),
						AttemptPath:      fmt.Sprintf("%d", i),
						TriggerReason:    "concurrent_test",
						Status:           "in_progress",
						StartTime:        time.Now(),
					}

					if err := mockConsul.AddHealingAttempt(ctx, transformID, attempt.AttemptPath, attempt); err != nil {
						atomic.AddInt64(&failureCount, 1)
						break
					}

					// Simulate work
					time.Sleep(10 * time.Millisecond)

					// Complete attempt
					attempt.Status = "completed"
					attempt.Result = "success"
					attempt.EndTime = time.Now()

					if err := mockConsul.UpdateHealingAttempt(ctx, transformID, attempt.AttemptPath, attempt); err != nil {
						atomic.AddInt64(&failureCount, 1)
						break
					}
				}

				// Mark transformation as completed
				if err := mockConsul.UpdateWorkflowStage(ctx, transformID, "completed"); err != nil {
					atomic.AddInt64(&failureCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				duration := time.Since(start).Nanoseconds()
				atomic.AddInt64(&totalDuration, duration)
				atomic.AddInt64(&completedCount, 1)
			}
		}(w)
	}

	// Monitor progress
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			completed := atomic.LoadInt64(&completedCount)
			active := atomic.LoadInt32(&activeGoroutines)
			t.Logf("Progress: %d/%d completed, %d active workers", completed, numTransformations, active)
		}
	}()

	// Wait for completion
	wg.Wait()
	ticker.Stop()
	elapsed := time.Since(startTime)

	// Calculate metrics
	avgDuration := time.Duration(atomic.LoadInt64(&totalDuration) / int64(numTransformations))
	throughput := float64(numTransformations) / elapsed.Seconds()
	successRate := float64(atomic.LoadInt64(&successCount)) / float64(numTransformations) * 100

	// Report results
	t.Logf("=== Load Test Results ===")
	t.Logf("Total transformations: %d", numTransformations)
	t.Logf("Workers: %d", numWorkers)
	t.Logf("Duration: %v", elapsed)
	t.Logf("Throughput: %.2f transformations/sec", throughput)
	t.Logf("Average duration: %v", avgDuration)
	t.Logf("Success rate: %.2f%%", successRate)
	t.Logf("Failures: %d", atomic.LoadInt64(&failureCount))

	// Assertions
	assert.Greater(t, throughput, 1.0, "Throughput should be > 1 transformation/sec")
	assert.Greater(t, successRate, 90.0, "Success rate should be > 90%")

	// Check coordinator metrics
	metrics := coordinator.GetMetrics()
	assert.Greater(t, metrics.CompletedTasks, 0)
	assert.LessOrEqual(t, metrics.FailedTasks, int(numTransformations/10), "Failed tasks should be < 10%")
}

// TestLoadConsulKVPerformance tests Consul KV operations under load
func TestLoadConsulKVPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numOperations = 1000
		numWorkers    = 20
		payloadSize   = 1024 // 1KB payload
	)

	ctx := context.Background()
	mockConsul := NewMockConsulStore()

	// Metrics
	var (
		writeLatencies []time.Duration
		readLatencies  []time.Duration
		mu             sync.Mutex
		errors         int64
	)

	// Generate test data
	testData := make([]byte, payloadSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Worker function
	worker := func(operations int) {
		for i := 0; i < operations; i++ {
			transformID := uuid.New().String()

			// Write operation
			writeStart := time.Now()
			status := &TransformationStatus{
				TransformationID: transformID,
				Status:           "in_progress",
				WorkflowStage:    "healing",
				StartTime:        time.Now(),
				Error:            string(testData), // Use as payload
			}

			if err := mockConsul.StoreTransformationStatus(ctx, transformID, status); err != nil {
				atomic.AddInt64(&errors, 1)
				continue
			}
			writeLatency := time.Since(writeStart)

			// Read operation
			readStart := time.Now()
			retrieved, err := mockConsul.GetTransformationStatus(ctx, transformID)
			if err != nil {
				atomic.AddInt64(&errors, 1)
				continue
			}
			readLatency := time.Since(readStart)

			// Verify data integrity
			if retrieved.TransformationID != transformID {
				atomic.AddInt64(&errors, 1)
			}

			// Record latencies
			mu.Lock()
			writeLatencies = append(writeLatencies, writeLatency)
			readLatencies = append(readLatencies, readLatency)
			mu.Unlock()
		}
	}

	// Run load test
	startTime := time.Now()
	var wg sync.WaitGroup

	operationsPerWorker := numOperations / numWorkers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(operationsPerWorker)
		}()
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	// Calculate statistics
	avgWriteLatency := calculateAverage(writeLatencies)
	avgReadLatency := calculateAverage(readLatencies)
	p95WriteLatency := calculatePercentile(writeLatencies, 95)
	p95ReadLatency := calculatePercentile(readLatencies, 95)
	throughput := float64(numOperations) / elapsed.Seconds()
	errorRate := float64(atomic.LoadInt64(&errors)) / float64(numOperations) * 100

	// Report results
	t.Logf("=== Consul KV Load Test Results ===")
	t.Logf("Total operations: %d", numOperations)
	t.Logf("Workers: %d", numWorkers)
	t.Logf("Duration: %v", elapsed)
	t.Logf("Throughput: %.2f ops/sec", throughput)
	t.Logf("Avg write latency: %v", avgWriteLatency)
	t.Logf("P95 write latency: %v", p95WriteLatency)
	t.Logf("Avg read latency: %v", avgReadLatency)
	t.Logf("P95 read latency: %v", p95ReadLatency)
	t.Logf("Error rate: %.2f%%", errorRate)

	// Assertions
	assert.Less(t, avgWriteLatency, 10*time.Millisecond, "Avg write latency should be < 10ms")
	assert.Less(t, avgReadLatency, 5*time.Millisecond, "Avg read latency should be < 5ms")
	assert.Less(t, errorRate, 1.0, "Error rate should be < 1%")
	assert.Greater(t, throughput, 100.0, "Throughput should be > 100 ops/sec")
}

// TestLoadLLMRateLimiting tests LLM API rate limiting and cost optimization
func TestLoadLLMRateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		requestsPerSecond = 10
		testDuration      = 10 * time.Second
		maxCostPerRequest = 0.05 // $0.05 per request
	)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Create cost tracker with rate limiting
	costTracker := NewLLMCostTracker(&LLMBudgetConfig{
		Enabled:               true,
		MaxCostPerTransform:   10.0,
		MaxCostPerHour:        5.0,
		MaxCostPerDay:         50.0,
		MaxCostPerMonth:       100.0,
		AlertThresholdPercent: 80.0,
		BlockOnExceed:         false,
	})

	// Metrics
	var (
		totalRequests  int64
		rateLimited    int64
		cacheHits      int64
		totalCost      float64
		costMu         sync.Mutex
		requestTimes   []time.Time
		requestTimesMu sync.Mutex
	)

	// Simulate requests
	ticker := time.NewTicker(time.Second / time.Duration(requestsPerSecond))
	defer ticker.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				atomic.AddInt64(&totalRequests, 1)

				// Check rate limit
				requestTimesMu.Lock()
				now := time.Now()
				// Remove old requests (older than 1 second)
				cutoff := now.Add(-time.Second)
				for len(requestTimes) > 0 && requestTimes[0].Before(cutoff) {
					requestTimes = requestTimes[1:]
				}

				if len(requestTimes) >= requestsPerSecond {
					atomic.AddInt64(&rateLimited, 1)
					requestTimesMu.Unlock()
					continue
				}
				requestTimes = append(requestTimes, now)
				requestTimesMu.Unlock()

				// Simulate LLM request
				promptTokens := 100 + (atomic.LoadInt64(&totalRequests) % 500)
				completionTokens := 50 + (atomic.LoadInt64(&totalRequests) % 200)

				// Check cache (simple simulation)
				if atomic.LoadInt64(&totalRequests)%10 < 3 { // 30% cache hit rate
					atomic.AddInt64(&cacheHits, 1)
					continue
				}

				// Record usage
				inputCost := float64(promptTokens) * 0.00003      // $0.03 per 1K tokens
				outputCost := float64(completionTokens) * 0.00006 // $0.06 per 1K tokens
				totalCostItem := inputCost + outputCost

				usage := LLMUsageRecord{
					ID:           uuid.New().String(),
					Model:        "gpt-4",
					Provider:     ProviderOpenAI,
					InputTokens:  int(promptTokens),
					OutputTokens: int(completionTokens),
					TotalTokens:  int(promptTokens + completionTokens),
					InputCost:    inputCost,
					OutputCost:   outputCost,
					TotalCost:    totalCostItem,
					TransformID:  uuid.New().String(),
					Timestamp:    time.Now(),
					CacheHit:     false,
				}

				err := costTracker.RecordUsage(ctx, usage)
				if err != nil {
					atomic.AddInt64(&rateLimited, 1)
				}

				costMu.Lock()
				totalCost += totalCostItem
				costMu.Unlock()
			}
		}
	}()

	// Monitor cost alerts
	alertChan := make(chan string, 10)
	go func() {
		for alert := range alertChan {
			t.Logf("Cost alert: %s", alert)
		}
	}()

	// Wait for test to complete
	wg.Wait()

	// Calculate metrics
	actualRequests := atomic.LoadInt64(&totalRequests) - atomic.LoadInt64(&rateLimited)
	effectiveRPS := float64(actualRequests) / testDuration.Seconds()
	cacheHitRate := float64(atomic.LoadInt64(&cacheHits)) / float64(atomic.LoadInt64(&totalRequests)) * 100
	avgCostPerRequest := totalCost / float64(actualRequests)

	// Get cost tracker metrics
	metrics := costTracker.GetMetrics()

	// Report results
	t.Logf("=== LLM Rate Limiting Load Test Results ===")
	t.Logf("Test duration: %v", testDuration)
	t.Logf("Target RPS: %d", requestsPerSecond)
	t.Logf("Effective RPS: %.2f", effectiveRPS)
	t.Logf("Total requests: %d", atomic.LoadInt64(&totalRequests))
	t.Logf("Rate limited: %d", atomic.LoadInt64(&rateLimited))
	t.Logf("Cache hits: %d (%.2f%%)", atomic.LoadInt64(&cacheHits), cacheHitRate)
	t.Logf("Total cost: $%.4f", totalCost)
	t.Logf("Avg cost per request: $%.6f", avgCostPerRequest)
	t.Logf("Total cost from metrics: $%.2f", metrics.TotalCost)

	// Assertions
	assert.LessOrEqual(t, effectiveRPS, float64(requestsPerSecond), "Should respect rate limit")
	assert.Less(t, avgCostPerRequest, maxCostPerRequest, "Cost per request should be optimized")
	assert.Greater(t, cacheHitRate, 20.0, "Cache hit rate should be > 20%")
}

// TestLoadMemoryUsageDeepHierarchies tests memory usage with deep healing hierarchies
func TestLoadMemoryUsageDeepHierarchies(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		maxDepth        = 10
		branchingFactor = 3
		numTrees        = 100
	)

	ctx := context.Background()
	mockConsul := NewMockConsulStore()

	// Get initial memory stats
	var initialMem runtime.MemStats
	runtime.ReadMemStats(&initialMem)

	// Create deep hierarchies
	var transformations []*TransformationStatus
	for tree := 0; tree < numTrees; tree++ {
		transformID := uuid.New().String()
		status := &TransformationStatus{
			TransformationID: transformID,
			Status:           "in_progress",
			WorkflowStage:    "healing",
			StartTime:        time.Now(),
			Children:         buildDeepHierarchy(1, maxDepth, branchingFactor, ""),
		}
		transformations = append(transformations, status)

		// Store in Consul
		err := mockConsul.StoreTransformationStatus(ctx, transformID, status)
		require.NoError(t, err)
	}

	// Force garbage collection
	runtime.GC()
	runtime.Gosched()

	// Get memory stats after creating hierarchies
	var afterCreateMem runtime.MemStats
	runtime.ReadMemStats(&afterCreateMem)

	// Calculate total nodes
	totalNodes := calculateTotalNodes(maxDepth, branchingFactor) * numTrees

	// Traverse all hierarchies (simulate processing)
	startTime := time.Now()
	nodeCount := 0
	for _, status := range transformations {
		nodeCount += traverseHierarchy(status.Children)
	}
	traversalTime := time.Since(startTime)

	// Clear references and run GC
	transformations = nil
	runtime.GC()
	runtime.Gosched()

	// Get final memory stats
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)

	// Calculate memory metrics
	memoryUsed := afterCreateMem.Alloc - initialMem.Alloc
	memoryPerNode := memoryUsed / uint64(totalNodes)
	memoryReclaimed := afterCreateMem.Alloc - finalMem.Alloc
	reclamationRate := float64(memoryReclaimed) / float64(afterCreateMem.Alloc-initialMem.Alloc) * 100

	// Report results
	t.Logf("=== Memory Usage Load Test Results ===")
	t.Logf("Trees created: %d", numTrees)
	t.Logf("Max depth: %d", maxDepth)
	t.Logf("Branching factor: %d", branchingFactor)
	t.Logf("Total nodes: %d", totalNodes)
	t.Logf("Memory used: %.2f MB", float64(memoryUsed)/(1024*1024))
	t.Logf("Memory per node: %d bytes", memoryPerNode)
	t.Logf("Traversal time: %v", traversalTime)
	t.Logf("Traversal rate: %.2f nodes/ms", float64(nodeCount)/float64(traversalTime.Milliseconds()))
	t.Logf("Memory reclaimed: %.2f MB (%.2f%%)", float64(memoryReclaimed)/(1024*1024), reclamationRate)

	// Assertions
	assert.Less(t, memoryPerNode, uint64(1024), "Memory per node should be < 1KB")
	assert.Greater(t, reclamationRate, 80.0, "Should reclaim > 80% of memory after GC")
	assert.Less(t, traversalTime, 5*time.Second, "Traversal should complete in < 5s")
}

// BenchmarkHealingWorkflow benchmarks the complete healing workflow
func BenchmarkHealingWorkflow(b *testing.B) {
	ctx := context.Background()
	mockConsul := NewMockConsulStore()

	config := DefaultHealingConfig()
	coordinator := NewHealingCoordinator(config)
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			transformID := uuid.New().String()

			// Create transformation
			status := &TransformationStatus{
				TransformationID: transformID,
				Status:           "failed",
				WorkflowStage:    "build",
				StartTime:        time.Now(),
				Error:            "Benchmark error",
			}

			// Store status
			_ = mockConsul.StoreTransformationStatus(ctx, transformID, status)

			// Add healing attempt
			attempt := &HealingAttempt{
				TransformationID: uuid.New().String(),
				AttemptPath:      "1",
				TriggerReason:    "benchmark",
				Status:           "in_progress",
				StartTime:        time.Now(),
			}
			_ = mockConsul.AddHealingAttempt(ctx, transformID, "1", attempt)

			// Complete healing
			attempt.Status = "completed"
			attempt.Result = "success"
			attempt.EndTime = time.Now()
			_ = mockConsul.UpdateHealingAttempt(ctx, transformID, "1", attempt)

			// Update workflow stage
			_ = mockConsul.UpdateWorkflowStage(ctx, transformID, "completed")
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "workflows/sec")
}

// BenchmarkConsulOperations benchmarks Consul KV operations
func BenchmarkConsulOperations(b *testing.B) {
	ctx := context.Background()
	mockConsul := NewMockConsulStore()

	// Pre-generate IDs
	ids := make([]string, b.N)
	for i := range ids {
		ids[i] = uuid.New().String()
	}

	b.Run("Store", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			status := &TransformationStatus{
				TransformationID: ids[i],
				Status:           "in_progress",
				StartTime:        time.Now(),
			}
			_ = mockConsul.StoreTransformationStatus(ctx, ids[i], status)
		}
	})

	b.Run("Get", func(b *testing.B) {
		// Pre-store data
		for i := 0; i < b.N; i++ {
			status := &TransformationStatus{
				TransformationID: ids[i],
				Status:           "in_progress",
				StartTime:        time.Now(),
			}
			_ = mockConsul.StoreTransformationStatus(ctx, ids[i], status)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = mockConsul.GetTransformationStatus(ctx, ids[i])
		}
	})

	b.Run("Update", func(b *testing.B) {
		// Pre-store data
		for i := 0; i < b.N; i++ {
			status := &TransformationStatus{
				TransformationID: ids[i],
				Status:           "in_progress",
				WorkflowStage:    "build",
				StartTime:        time.Now(),
			}
			_ = mockConsul.StoreTransformationStatus(ctx, ids[i], status)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = mockConsul.UpdateWorkflowStage(ctx, ids[i], "completed")
		}
	})
}

// Helper functions

func buildDeepHierarchy(currentDepth, maxDepth, branchingFactor int, parentPath string) []HealingAttempt {
	if currentDepth > maxDepth {
		return nil
	}

	var attempts []HealingAttempt
	for i := 0; i < branchingFactor; i++ {
		attemptPath := fmt.Sprintf("%s%d", parentPath, i+1)
		if parentPath != "" {
			attemptPath = parentPath + "." + fmt.Sprintf("%d", i+1)
		}

		attempt := HealingAttempt{
			TransformationID: uuid.New().String(),
			AttemptPath:      attemptPath,
			TriggerReason:    "load_test",
			Status:           "completed",
			Result:           "success",
			StartTime:        time.Now(),
			EndTime:          time.Now(),
			Children:         buildDeepHierarchy(currentDepth+1, maxDepth, branchingFactor, attemptPath),
		}
		attempts = append(attempts, attempt)
	}
	return attempts
}

func calculateTotalNodes(depth, branchingFactor int) int {
	// Geometric series: (b^(d+1) - 1) / (b - 1)
	if branchingFactor == 1 {
		return depth
	}
	total := 1
	for i := 1; i <= depth; i++ {
		total = total*branchingFactor + 1
	}
	return total
}

func traverseHierarchy(attempts []HealingAttempt) int {
	count := len(attempts)
	for _, attempt := range attempts {
		count += traverseHierarchy(attempt.Children)
	}
	return count
}

func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

func calculatePercentile(durations []time.Duration, percentile int) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	// Simple percentile calculation (not exact for small samples)
	index := len(durations) * percentile / 100
	if index >= len(durations) {
		index = len(durations) - 1
	}
	return durations[index]
}
