//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealingCoordinator_ConcurrencyControl(t *testing.T) {
	tests := []struct {
		name             string
		maxParallel      int
		totalTasks       int
		expectedParallel int
		taskDuration     time.Duration
	}{
		{
			name:             "respects max parallel limit",
			maxParallel:      3,
			totalTasks:       10,
			expectedParallel: 3,
			taskDuration:     100 * time.Millisecond,
		},
		{
			name:             "handles single worker",
			maxParallel:      1,
			totalTasks:       5,
			expectedParallel: 1,
			taskDuration:     50 * time.Millisecond,
		},
		{
			name:             "handles more workers than tasks",
			maxParallel:      10,
			totalTasks:       3,
			expectedParallel: 3,
			taskDuration:     100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := NewHealingCoordinator(&HealingConfig{
				MaxParallelAttempts: tt.maxParallel,
				MaxTotalAttempts:    100,
				HealingTimeout:      10 * time.Second,
				MaxHealingDepth:     5, // Set proper depth limit
				QueueSize:           100,
			})

			// Start the coordinator
			ctx := context.Background()
			err := coordinator.Start(ctx)
			require.NoError(t, err)
			defer coordinator.Stop()

			// Track concurrent executions
			var currentParallel int32
			var maxObservedParallel int32
			var completedTasks int32

			// Submit tasks
			for i := 0; i < tt.totalTasks; i++ {
				task := &HealingTask{
					TransformID: fmt.Sprintf("transform-%d", i),
					AttemptPath: fmt.Sprintf("%d", i),
					Priority:    i,
					ExecuteFn: func(ctx context.Context) error {
						// Increment parallel counter
						current := atomic.AddInt32(&currentParallel, 1)

						// Track max parallel
						for {
							max := atomic.LoadInt32(&maxObservedParallel)
							if current <= max || atomic.CompareAndSwapInt32(&maxObservedParallel, max, current) {
								break
							}
						}

						// Simulate work
						time.Sleep(tt.taskDuration)

						// Decrement parallel counter
						atomic.AddInt32(&currentParallel, -1)
						atomic.AddInt32(&completedTasks, 1)
						return nil
					},
				}

				err := coordinator.SubmitTask(ctx, task)
				assert.NoError(t, err)
			}

			// Wait for all tasks to complete
			require.Eventually(t, func() bool {
				return atomic.LoadInt32(&completedTasks) == int32(tt.totalTasks)
			}, 5*time.Second, 10*time.Millisecond)

			// Verify max parallel was respected
			assert.LessOrEqual(t, int(maxObservedParallel), tt.expectedParallel,
				"Max parallel executions exceeded limit")
		})
	}
}

func TestHealingCoordinator_QueueManagement(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 1,
		MaxTotalAttempts:    10,
		QueueSize:           100,
		MaxHealingDepth:     5,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	var completed int32

	// Submit multiple tasks quickly
	for i := 0; i < 5; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("task-%d", i),
			AttemptPath: fmt.Sprintf("%d", i),
			ExecuteFn: func(ctx context.Context) error {
				time.Sleep(10 * time.Millisecond) // Short delay
				atomic.AddInt32(&completed, 1)
				return nil
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		require.NoError(t, err)
	}

	// Verify basic operation
	metrics := coordinator.GetMetrics()
	assert.Equal(t, 5, metrics.TotalSubmitted, "Should have 5 submitted tasks")

	// Wait for all tasks to complete
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&completed) == 5
	}, 2*time.Second, 10*time.Millisecond, "All tasks should complete")

	finalMetrics := coordinator.GetMetrics()
	assert.Equal(t, 5, finalMetrics.CompletedTasks, "Should have 5 completed tasks")
	assert.Equal(t, 0, finalMetrics.ActiveWorkers, "Should have no active workers")
}

func TestHealingCoordinator_PriorityQueue(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 1, // Single worker to ensure order
		MaxTotalAttempts:    10,
		MaxHealingDepth:     5,
		QueueSize:           100,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Block the worker
	blockDone := make(chan struct{})
	blockTask := &HealingTask{
		TransformID: "blocker",
		AttemptPath: "blocker", // Add attempt path
		Priority:    100,
		ExecuteFn: func(ctx context.Context) error {
			<-blockDone
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, blockTask)
	require.NoError(t, err)

	// Submit tasks with different priorities
	var executionOrder []string
	var orderMutex sync.Mutex

	tasks := []struct {
		id       string
		priority int
	}{
		{"low-priority", 10},
		{"high-priority", 1},
		{"medium-priority", 5},
		{"urgent", 0},
	}

	for _, task := range tasks {
		taskItem := task // Capture for closure
		healingTask := &HealingTask{
			TransformID: taskItem.id,
			AttemptPath: taskItem.id, // Use id as attempt path
			Priority:    taskItem.priority,
			ExecuteFn: func(ctx context.Context) error {
				orderMutex.Lock()
				executionOrder = append(executionOrder, taskItem.id)
				orderMutex.Unlock()
				return nil
			},
		}
		err := coordinator.SubmitTask(ctx, healingTask)
		assert.NoError(t, err)
	}

	// Unblock the worker
	close(blockDone)

	// Wait for all tasks to complete
	require.Eventually(t, func() bool {
		orderMutex.Lock()
		defer orderMutex.Unlock()
		return len(executionOrder) == 4
	}, 2*time.Second, 10*time.Millisecond)

	// Verify execution order (lower priority value = higher priority)
	assert.Equal(t, []string{"urgent", "high-priority", "medium-priority", "low-priority"}, executionOrder)
}

func TestHealingCoordinator_MaxAttemptsEnforcement(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 5,
		MaxTotalAttempts:    3,
		MaxHealingDepth:     5,
		QueueSize:           100,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit tasks up to the limit
	for i := 0; i < 3; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("task-%d", i),
			AttemptPath: fmt.Sprintf("%d", i),
			ExecuteFn: func(ctx context.Context) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Try to submit one more - should fail
	task := &HealingTask{
		TransformID: "exceeds-limit",
		AttemptPath: "4",
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max total attempts")
}

func TestHealingCoordinator_DepthLimitEnforcement(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 5,
		MaxTotalAttempts:    100,
		MaxHealingDepth:     3,
		QueueSize:           100,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit task at max depth
	task := &HealingTask{
		TransformID: "deep-task",
		AttemptPath: "1.1.1", // Depth 3
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, task)
	assert.NoError(t, err)

	// Try to submit deeper task - should fail
	deeperTask := &HealingTask{
		TransformID: "too-deep",
		AttemptPath: "1.1.1.1", // Depth 4
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, deeperTask)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max healing depth")
}

func TestHealingCoordinator_GracefulShutdown(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    10,
		MaxHealingDepth:     5,
		QueueSize:           100,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)

	// Submit tasks that will be running during shutdown
	var completed int32
	for i := 0; i < 5; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("task-%d", i),
			ExecuteFn: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					atomic.AddInt32(&completed, 1)
					return nil
				}
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Give tasks time to start
	time.Sleep(50 * time.Millisecond)

	// Initiate graceful shutdown
	shutdownComplete := make(chan struct{})
	go func() {
		coordinator.Stop()
		close(shutdownComplete)
	}()

	// Shutdown should wait for active tasks
	select {
	case <-shutdownComplete:
		// Check that some tasks completed
		assert.Greater(t, atomic.LoadInt32(&completed), int32(0))
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown took too long")
	}

	// Verify coordinator is stopped
	task := &HealingTask{
		TransformID: "after-shutdown",
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator stopped")
}

func TestHealingCoordinator_Metrics(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 3,
		MaxTotalAttempts:    100,
		MaxHealingDepth:     5,
		QueueSize:           100,
	})

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Initial metrics
	metrics := coordinator.GetMetrics()
	assert.Equal(t, 0, metrics.ActiveWorkers)
	assert.Equal(t, 0, metrics.QueuedTasks)
	assert.Equal(t, 0, metrics.CompletedTasks)
	assert.Equal(t, 0, metrics.FailedTasks)

	// Submit tasks
	var taskWg sync.WaitGroup
	taskWg.Add(5)

	for i := 0; i < 5; i++ {
		shouldFail := i%2 == 0
		task := &HealingTask{
			TransformID: fmt.Sprintf("task-%d", i),
			ExecuteFn: func(ctx context.Context) error {
				defer taskWg.Done()
				time.Sleep(50 * time.Millisecond)
				if shouldFail {
					return fmt.Errorf("simulated failure")
				}
				return nil
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Check metrics during execution
	time.Sleep(25 * time.Millisecond)
	metrics = coordinator.GetMetrics()
	assert.Greater(t, metrics.ActiveWorkers, 0)
	assert.LessOrEqual(t, metrics.ActiveWorkers, 3)

	// Wait for completion
	taskWg.Wait()

	// Final metrics
	metrics = coordinator.GetMetrics()
	assert.Equal(t, 0, metrics.ActiveWorkers)
	assert.Equal(t, 0, metrics.QueuedTasks)
	assert.Equal(t, 2, metrics.CompletedTasks) // 2 successful
	assert.Equal(t, 3, metrics.FailedTasks)    // 3 failed
	assert.Equal(t, 5, metrics.TotalSubmitted)
}

func TestHealingCoordinator_ContextCancellation(t *testing.T) {
	coordinator := NewHealingCoordinator(&HealingConfig{
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    10,
		MaxHealingDepth:     5,
		QueueSize:           100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit tasks
	var cancelled int32
	for i := 0; i < 5; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("task-%d", i),
			ExecuteFn: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					atomic.AddInt32(&cancelled, 1)
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return nil
				}
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Cancel context after tasks start
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Check that tasks were cancelled
	assert.Greater(t, atomic.LoadInt32(&cancelled), int32(0))

	// New submissions should fail
	task := &HealingTask{
		TransformID: "after-cancel",
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}
	err = coordinator.SubmitTask(ctx, task)
	assert.Error(t, err)
}

// TestCircuitBreaker tests the circuit breaker functionality
func TestCircuitBreaker(t *testing.T) {
	t.Run("opens after threshold failures", func(t *testing.T) {
		config := &HealingConfig{
			FailureThreshold:    3,
			CircuitOpenDuration: 100 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		// Should be closed initially
		assert.True(t, cb.CanExecute())
		state, failures, _ := cb.GetState()
		assert.Equal(t, "closed", state)
		assert.Equal(t, 0, failures)

		// Record failures up to threshold
		cb.RecordFailure()
		assert.True(t, cb.CanExecute()) // Still closed after 1 failure

		cb.RecordFailure()
		assert.True(t, cb.CanExecute()) // Still closed after 2 failures

		cb.RecordFailure()
		assert.False(t, cb.CanExecute()) // Should be open after 3 failures

		state, failures, openUntil := cb.GetState()
		assert.Equal(t, "open", state)
		assert.Equal(t, 3, failures)
		assert.False(t, openUntil.IsZero())
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		config := &HealingConfig{
			FailureThreshold:    2,
			CircuitOpenDuration: 50 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()
		assert.False(t, cb.CanExecute())

		// Wait for circuit open duration
		time.Sleep(60 * time.Millisecond)

		// Should transition to half-open and allow one attempt
		assert.True(t, cb.CanExecute())
		state, _, _ := cb.GetState()
		assert.Equal(t, "half-open", state)
	})

	t.Run("closes on success in half-open state", func(t *testing.T) {
		config := &HealingConfig{
			FailureThreshold:    2,
			CircuitOpenDuration: 50 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()

		// Wait for half-open
		time.Sleep(60 * time.Millisecond)
		assert.True(t, cb.CanExecute())

		// Record success
		cb.RecordSuccess()

		// Should be closed now
		state, failures, _ := cb.GetState()
		assert.Equal(t, "closed", state)
		assert.Equal(t, 0, failures)
	})

	t.Run("reopens on failure in half-open state", func(t *testing.T) {
		config := &HealingConfig{
			FailureThreshold:    2,
			CircuitOpenDuration: 50 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		// Open the circuit
		cb.RecordFailure()
		cb.RecordFailure()

		// Wait for half-open
		time.Sleep(60 * time.Millisecond)
		assert.True(t, cb.CanExecute())

		// Record failure in half-open
		cb.RecordFailure()

		// Should be open again
		assert.False(t, cb.CanExecute())
		state, _, _ := cb.GetState()
		assert.Equal(t, "open", state)
	})
}

// TestHealingCoordinator_DepthLimits tests depth limit enforcement
func TestHealingCoordinator_DepthLimits(t *testing.T) {
	config := &HealingConfig{
		MaxHealingDepth:     3,
		MaxParallelAttempts: 5,
		MaxTotalAttempts:    100,
		AttemptTimeout:      1 * time.Second,
		FailureThreshold:    10,
		CircuitOpenDuration: 1 * time.Minute,
	}

	coordinator := NewHealingCoordinator(config)
	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	t.Run("accepts tasks within depth limit", func(t *testing.T) {
		validPaths := []string{"1", "1.1", "1.1.1"}

		for _, path := range validPaths {
			task := &HealingTask{
				TransformID: fmt.Sprintf("transform-%s", path),
				AttemptPath: path,
				ExecuteFn: func(ctx context.Context) error {
					return nil
				},
			}

			err := coordinator.SubmitTask(ctx, task)
			assert.NoError(t, err, "should accept path %s", path)
		}
	})

	t.Run("rejects tasks exceeding depth limit", func(t *testing.T) {
		deepPaths := []string{"1.1.1.1", "2.3.4.5", "1.2.3.4.5"}

		for _, path := range deepPaths {
			task := &HealingTask{
				TransformID: fmt.Sprintf("transform-%s", path),
				AttemptPath: path,
				ExecuteFn: func(ctx context.Context) error {
					return nil
				},
			}

			err := coordinator.SubmitTask(ctx, task)
			assert.Error(t, err, "should reject path %s", path)
			assert.Contains(t, err.Error(), "max healing depth")
		}

		// Check metrics
		metrics := coordinator.GetMetrics()
		assert.Greater(t, metrics.DepthLimitReached, 0)
	})
}

// TestHealingCoordinator_TotalAttemptsLimit tests total attempts limit enforcement
func TestHealingCoordinator_TotalAttemptsLimit(t *testing.T) {
	config := &HealingConfig{
		MaxHealingDepth:     5,
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    5,
		AttemptTimeout:      1 * time.Second,
		FailureThreshold:    10,
		CircuitOpenDuration: 1 * time.Minute,
	}

	coordinator := NewHealingCoordinator(config)
	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	transformID := "test-transform"

	// Submit tasks up to the limit
	for i := 0; i < config.MaxTotalAttempts; i++ {
		task := &HealingTask{
			TransformID: transformID,
			AttemptPath: fmt.Sprintf("%d", i+1),
			ExecuteFn: func(ctx context.Context) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		}

		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err, "should accept attempt %d", i+1)
	}

	// Next attempt should be rejected
	task := &HealingTask{
		TransformID: transformID,
		AttemptPath: "6",
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}

	err = coordinator.SubmitTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max total attempts")

	// Check metrics
	metrics := coordinator.GetMetrics()
	assert.Greater(t, metrics.AttemptsLimitReached, 0)
}

// TestHealingCoordinator_CircuitBreakerIntegration tests circuit breaker integration
func TestHealingCoordinator_CircuitBreakerIntegration(t *testing.T) {
	config := &HealingConfig{
		MaxHealingDepth:     5,
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    100,
		AttemptTimeout:      100 * time.Millisecond,
		FailureThreshold:    3,
		CircuitOpenDuration: 200 * time.Millisecond,
	}

	coordinator := NewHealingCoordinator(config)
	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit failing tasks to trip the circuit breaker
	var failCount int32
	for i := 0; i < config.FailureThreshold; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("fail-%d", i),
			AttemptPath: fmt.Sprintf("%d", i+1),
			ExecuteFn: func(ctx context.Context) error {
				atomic.AddInt32(&failCount, 1)
				return fmt.Errorf("intentional failure")
			},
		}

		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Wait for tasks to fail
	time.Sleep(150 * time.Millisecond)

	// Circuit should be open now, new submissions should fail
	task := &HealingTask{
		TransformID: "should-be-rejected",
		AttemptPath: "1",
		ExecuteFn: func(ctx context.Context) error {
			return nil
		},
	}

	err = coordinator.SubmitTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")

	// Check metrics
	metrics := coordinator.GetMetrics()
	assert.Equal(t, "open", metrics.CircuitBreakerState)
	assert.Equal(t, config.FailureThreshold, metrics.ConsecutiveFailures)

	// Wait for circuit to potentially reset to half-open
	time.Sleep(config.CircuitOpenDuration + 50*time.Millisecond)

	// Should allow one attempt now (half-open)
	successTask := &HealingTask{
		TransformID: "recovery-attempt",
		AttemptPath: "1",
		ExecuteFn: func(ctx context.Context) error {
			return nil // Success to close the circuit
		},
	}

	err = coordinator.SubmitTask(ctx, successTask)
	assert.NoError(t, err)

	// Wait for task to complete
	time.Sleep(50 * time.Millisecond)

	// Circuit should be closed now
	metrics = coordinator.GetMetrics()
	assert.Equal(t, "closed", metrics.CircuitBreakerState)
}

// TestHealingCoordinator_PerformanceMetrics tests performance metrics tracking
func TestHealingCoordinator_PerformanceMetrics(t *testing.T) {
	config := &HealingConfig{
		MaxHealingDepth:     5,
		MaxParallelAttempts: 3,
		MaxTotalAttempts:    100,
		AttemptTimeout:      1 * time.Second,
		FailureThreshold:    10,
		CircuitOpenDuration: 1 * time.Minute,
	}

	coordinator := NewHealingCoordinator(config)
	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit mix of successful and failing tasks
	successCount := 7
	failCount := 3

	for i := 0; i < successCount; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("success-%d", i),
			AttemptPath: fmt.Sprintf("%d", i+1),
			ExecuteFn: func(ctx context.Context) error {
				time.Sleep(50 * time.Millisecond)
				return nil
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	for i := 0; i < failCount; i++ {
		task := &HealingTask{
			TransformID: fmt.Sprintf("fail-%d", i),
			AttemptPath: fmt.Sprintf("%d", i+1),
			ExecuteFn: func(ctx context.Context) error {
				time.Sleep(30 * time.Millisecond)
				return fmt.Errorf("failure")
			},
		}
		err := coordinator.SubmitTask(ctx, task)
		assert.NoError(t, err)
	}

	// Wait for all tasks to complete
	time.Sleep(200 * time.Millisecond)

	// Check metrics
	metrics := coordinator.GetMetrics()

	// Verify counts
	assert.Equal(t, successCount+failCount, metrics.TotalSubmitted)
	assert.Equal(t, successCount, metrics.CompletedTasks)
	assert.Equal(t, failCount, metrics.FailedTasks)

	// Verify success rate
	expectedRate := float64(successCount) / float64(successCount+failCount)
	assert.InDelta(t, expectedRate, metrics.SuccessRate, 0.01)

	// Verify average duration is calculated
	assert.Greater(t, metrics.AverageHealingDuration, time.Duration(0))
	assert.Greater(t, metrics.TotalHealingTime, time.Duration(0))

	// Verify timing makes sense
	assert.True(t, metrics.AverageHealingDuration > 30*time.Millisecond)
	assert.True(t, metrics.AverageHealingDuration < 100*time.Millisecond)
}

// TestHealingCoordinator_TimeoutTracking tests timeout tracking
func TestHealingCoordinator_TimeoutTracking(t *testing.T) {
	config := &HealingConfig{
		MaxHealingDepth:     5,
		MaxParallelAttempts: 2,
		MaxTotalAttempts:    100,
		AttemptTimeout:      50 * time.Millisecond, // Short timeout
		FailureThreshold:    10,
		CircuitOpenDuration: 1 * time.Minute,
	}

	coordinator := NewHealingCoordinator(config)
	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)
	defer coordinator.Stop()

	// Submit task that will timeout
	task := &HealingTask{
		TransformID: "timeout-task",
		AttemptPath: "1",
		ExecuteFn: func(ctx context.Context) error {
			select {
			case <-time.After(200 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	err = coordinator.SubmitTask(ctx, task)
	assert.NoError(t, err)

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Check metrics
	metrics := coordinator.GetMetrics()
	assert.Greater(t, metrics.TimeoutExceeded, 0)
	assert.Greater(t, metrics.FailedTasks, 0)
}
