package arf

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerBasicOperation(t *testing.T) {
	config := CircuitConfig{
		FailureThreshold:  3,
		OpenTimeout:       100 * time.Millisecond,
		MaxRetries:        2,
		BackoffMultiplier: 2.0,
		JitterEnabled:     false, // Disable jitter for predictable testing
		MinOpenDuration:   50 * time.Millisecond,
		MaxOpenDuration:   500 * time.Millisecond,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	t.Run("Successful operation", func(t *testing.T) {
		successOp := func() error { return nil }
		
		err := cb.Execute(ctx, successOp)
		if err != nil {
			t.Errorf("Expected no error for successful operation, got: %v", err)
		}

		if cb.GetState() != CircuitClosed {
			t.Errorf("Expected circuit to remain closed, got: %s", cb.GetState())
		}

		metrics := cb.GetMetrics()
		if metrics.SuccessCount != 1 {
			t.Errorf("Expected 1 success, got: %d", metrics.SuccessCount)
		}
	})

	t.Run("Circuit opens after threshold failures", func(t *testing.T) {
		cb.Reset() // Reset circuit to closed state
		
		failOp := func() error { return errors.New("operation failed") }

		// Execute failing operations up to threshold
		for i := 0; i < 3; i++ {
			err := cb.Execute(ctx, failOp)
			if err == nil {
				t.Errorf("Expected error from failing operation %d", i+1)
			}
		}

		// Circuit should now be open
		if cb.GetState() != CircuitOpen {
			t.Errorf("Expected circuit to be open after threshold failures, got: %s", cb.GetState())
		}

		metrics := cb.GetMetrics()
		if metrics.ConsecutiveFailures != 3 {
			t.Errorf("Expected 3 consecutive failures, got: %d", metrics.ConsecutiveFailures)
		}
	})

	t.Run("Circuit rejects requests when open", func(t *testing.T) {
		// Circuit should be open from previous test
		if cb.GetState() != CircuitOpen {
			t.Skip("Circuit not open, skipping rejection test")
		}

		successOp := func() error { return nil }
		
		err := cb.Execute(ctx, successOp)
		if err == nil {
			t.Error("Expected circuit breaker to reject request when open")
		}

		metrics := cb.GetMetrics()
		if metrics.RejectedRequests == 0 {
			t.Error("Expected rejected request count to increase")
		}
	})

	t.Run("Circuit transitions to half-open after timeout", func(t *testing.T) {
		cb.Reset() // Reset to start fresh
		
		// Force circuit to open
		failOp := func() error { return errors.New("operation failed") }
		for i := 0; i < 3; i++ {
			cb.Execute(ctx, failOp)
		}

		// Wait for open timeout
		time.Sleep(150 * time.Millisecond)

		// Next request should transition to half-open
		testOp := func() error { return nil }
		err := cb.Execute(ctx, testOp)

		if err != nil {
			t.Errorf("Expected successful operation in half-open state, got: %v", err)
		}

		// Should be closed again after successful operation
		if cb.GetState() != CircuitClosed {
			t.Errorf("Expected circuit to close after successful half-open operation, got: %s", cb.GetState())
		}
	})
}

func TestCircuitBreakerRetryLogic(t *testing.T) {
	config := CircuitConfig{
		FailureThreshold:  5,
		OpenTimeout:       100 * time.Millisecond,
		MaxRetries:        3,
		BackoffMultiplier: 1.5,
		JitterEnabled:     false,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	t.Run("Retries failing operation", func(t *testing.T) {
		attempts := 0
		retryOp := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil // Success on third attempt
		}

		startTime := time.Now()
		err := cb.Execute(ctx, retryOp)
		duration := time.Since(startTime)

		if err != nil {
			t.Errorf("Expected eventual success, got: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got: %d", attempts)
		}

		// Should take some time due to backoff
		if duration < 50*time.Millisecond {
			t.Errorf("Expected operation to take time for backoff, took: %v", duration)
		}
	})

	t.Run("Respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		slowOp := func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}

		err := cb.Execute(ctx, slowOp)
		if err == nil {
			t.Error("Expected context cancellation error")
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected deadline exceeded error, got: %v", err)
		}
	})
}

func TestCircuitBreakerMetrics(t *testing.T) {
	config := CircuitConfig{
		FailureThreshold:  3,
		OpenTimeout:       100 * time.Millisecond,
		MaxRetries:        1,
		BackoffMultiplier: 2.0,
		JitterEnabled:     false,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	t.Run("Tracks success metrics", func(t *testing.T) {
		successOp := func() error { return nil }

		for i := 0; i < 5; i++ {
			cb.Execute(ctx, successOp)
		}

		metrics := cb.GetMetrics()
		if metrics.SuccessCount != 5 {
			t.Errorf("Expected 5 successes, got: %d", metrics.SuccessCount)
		}

		if metrics.TotalRequests != 5 {
			t.Errorf("Expected 5 total requests, got: %d", metrics.TotalRequests)
		}

		if metrics.FailureRate != 0.0 {
			t.Errorf("Expected 0 failure rate, got: %f", metrics.FailureRate)
		}
	})

	t.Run("Tracks failure metrics", func(t *testing.T) {
		cb.Reset()
		
		failOp := func() error { return errors.New("failure") }

		for i := 0; i < 2; i++ {
			cb.Execute(ctx, failOp)
		}

		metrics := cb.GetMetrics()
		if metrics.FailureCount != 2 {
			t.Errorf("Expected 2 failures, got: %d", metrics.FailureCount)
		}

		if metrics.ConsecutiveFailures != 2 {
			t.Errorf("Expected 2 consecutive failures, got: %d", metrics.ConsecutiveFailures)
		}

		expectedFailureRate := 2.0 / 2.0 // 2 failures out of 2 total
		if metrics.FailureRate != expectedFailureRate {
			t.Errorf("Expected failure rate %f, got: %f", expectedFailureRate, metrics.FailureRate)
		}
	})

	t.Run("Tracks response time", func(t *testing.T) {
		cb.Reset()
		
		slowOp := func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		}

		cb.Execute(ctx, slowOp)

		metrics := cb.GetMetrics()
		if metrics.AvgResponseTime < 5*time.Millisecond {
			t.Errorf("Expected average response time to reflect operation duration, got: %v", metrics.AvgResponseTime)
		}
	})
}

func TestCircuitBreakerManager(t *testing.T) {
	manager := NewCircuitBreakerManager()

	t.Run("Creates circuit breakers on demand", func(t *testing.T) {
		cb1 := manager.GetCircuitBreaker("service1")
		cb2 := manager.GetCircuitBreaker("service2")

		if cb1 == cb2 {
			t.Error("Expected different circuit breakers for different services")
		}

		// Same service should return same circuit breaker
		cb1Again := manager.GetCircuitBreaker("service1")
		if cb1 != cb1Again {
			t.Error("Expected same circuit breaker for same service")
		}
	})

	t.Run("Uses custom configurations", func(t *testing.T) {
		customConfig := CircuitConfig{
			FailureThreshold:  5,
			OpenTimeout:       200 * time.Millisecond,
			MaxRetries:        5,
			BackoffMultiplier: 3.0,
		}

		manager.SetConfig("custom-service", customConfig)
		cb := manager.GetCircuitBreaker("custom-service")

		// Test that custom config is applied by checking behavior
		// This is indirect since we can't access config directly
		ctx := context.Background()
		failOp := func() error { return errors.New("failure") }

		// Should require 5 failures to open (custom threshold)
		for i := 0; i < 4; i++ {
			cb.Execute(ctx, failOp)
		}

		if cb.GetState() != CircuitClosed {
			t.Error("Circuit should still be closed with custom higher threshold")
		}

		// Fifth failure should open it
		cb.Execute(ctx, failOp)
		if cb.GetState() != CircuitOpen {
			t.Error("Circuit should be open after reaching custom threshold")
		}
	})

	t.Run("Returns metrics for all circuits", func(t *testing.T) {
		// Use existing circuit breakers from previous tests
		allMetrics := manager.GetAllMetrics()

		if len(allMetrics) < 2 {
			t.Errorf("Expected at least 2 circuit breakers, got: %d", len(allMetrics))
		}

		// Check that metrics contain expected services
		if _, exists := allMetrics["service1"]; !exists {
			t.Error("Expected metrics for service1")
		}

		if _, exists := allMetrics["custom-service"]; !exists {
			t.Error("Expected metrics for custom-service")
		}
	})
}

func TestCircuitBreakerEdgeCases(t *testing.T) {
	t.Run("Zero configuration values use defaults", func(t *testing.T) {
		config := CircuitConfig{} // All zero values
		cb := NewCircuitBreaker(config)

		// Should not panic and should use reasonable defaults
		ctx := context.Background()
		successOp := func() error { return nil }

		err := cb.Execute(ctx, successOp)
		if err != nil {
			t.Errorf("Expected circuit breaker with default config to work, got: %v", err)
		}
	})

	t.Run("Handles rapid state transitions", func(t *testing.T) {
		config := CircuitConfig{
			FailureThreshold:  1,
			OpenTimeout:       1 * time.Millisecond,
			MaxRetries:        0,
			BackoffMultiplier: 1.0,
		}

		cb := NewCircuitBreaker(config)
		ctx := context.Background()

		// Rapid failure -> open -> half-open -> closed cycle
		failOp := func() error { return errors.New("failure") }
		successOp := func() error { return nil }

		// Fail to open
		cb.Execute(ctx, failOp)
		if cb.GetState() != CircuitOpen {
			t.Error("Expected circuit to open after single failure")
		}

		// Wait and succeed to close
		time.Sleep(5 * time.Millisecond)
		err := cb.Execute(ctx, successOp)
		if err != nil {
			t.Errorf("Expected success after timeout, got: %v", err)
		}

		if cb.GetState() != CircuitClosed {
			t.Error("Expected circuit to close after successful operation")
		}
	})

	t.Run("Thread safety under concurrent access", func(t *testing.T) {
		config := CircuitConfig{
			FailureThreshold:  10,
			OpenTimeout:       100 * time.Millisecond,
			MaxRetries:        1,
		}

		cb := NewCircuitBreaker(config)
		ctx := context.Background()

		// Run concurrent operations
		done := make(chan bool, 10)
		
		for i := 0; i < 10; i++ {
			go func(id int) {
				defer func() { done <- true }()
				
				op := func() error {
					if id%2 == 0 {
						return nil // Success
					}
					return errors.New("failure") // Failure
				}

				cb.Execute(ctx, op)
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic and should have consistent metrics
		metrics := cb.GetMetrics()
		if metrics.TotalRequests != 10 {
			t.Errorf("Expected 10 total requests, got: %d", metrics.TotalRequests)
		}

		// Should have some successes and failures
		if metrics.SuccessCount == 0 || metrics.FailureCount == 0 {
			t.Error("Expected both successes and failures from concurrent operations")
		}
	})
}

// Benchmark circuit breaker performance
func BenchmarkCircuitBreaker(b *testing.B) {
	config := CircuitConfig{
		FailureThreshold:  100,
		OpenTimeout:       time.Second,
		MaxRetries:        0, // No retries for pure overhead measurement
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()
	successOp := func() error { return nil }

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(ctx, successOp)
		}
	})
}