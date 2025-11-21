package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	if policy.InitialInterval != 2*time.Second {
		t.Errorf("expected InitialInterval 2s, got %v", policy.InitialInterval)
	}
	if policy.MaxInterval != 30*time.Second {
		t.Errorf("expected MaxInterval 30s, got %v", policy.MaxInterval)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %v", policy.Multiplier)
	}
	if policy.MaxAttempts != 10 {
		t.Errorf("expected MaxAttempts 10, got %d", policy.MaxAttempts)
	}
}

func TestRetryWithBackoffSuccess(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     5,
	}

	attempts := 0
	err := RetryWithBackoff(ctx, policy, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffExhaustsAttempts(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	attempts := 0
	testErr := errors.New("persistent error")
	err := RetryWithBackoff(ctx, policy, func() error {
		attempts++
		return testErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected error to wrap testErr, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	policy := RetryPolicy{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     500 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     10,
	}

	attempts := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := RetryWithBackoff(ctx, policy, func() error {
		attempts++
		return errors.New("error")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts < 1 {
		t.Errorf("expected at least 1 attempt, got %d", attempts)
	}
}

func TestRetryWithBackoffImmediateSuccess(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     5,
	}

	attempts := 0
	err := RetryWithBackoff(ctx, policy, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestPollWithBackoffConditionMet(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     5,
	}

	attempts := 0
	err := PollWithBackoff(ctx, policy, nil, nil, "test_poll", func() (bool, error) {
		attempts++
		if attempts < 2 {
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestPollWithBackoffConditionNeverMet(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	attempts := 0
	err := PollWithBackoff(ctx, policy, nil, nil, "test_poll", func() (bool, error) {
		attempts++
		return false, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPollWithBackoffConditionError(t *testing.T) {
	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	attempts := 0
	testErr := errors.New("condition error")
	err := PollWithBackoff(ctx, policy, nil, nil, "test_poll", func() (bool, error) {
		attempts++
		return false, testErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error should wrap testErr and exhaust all attempts.
	if !strings.Contains(err.Error(), "condition error") {
		t.Errorf("expected error to contain 'condition error', got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (exhausted), got %d", attempts)
	}
}

func TestRetryWithBackoffExponentialIntervals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	ctx := context.Background()
	policy := RetryPolicy{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Multiplier:      2.0,
		MaxAttempts:     4,
	}

	var intervals []time.Duration
	lastAttempt := time.Now()
	attempts := 0

	err := RetryWithBackoff(ctx, policy, func() error {
		now := time.Now()
		if attempts > 0 {
			intervals = append(intervals, now.Sub(lastAttempt))
		}
		lastAttempt = now
		attempts++
		return errors.New("error")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// We expect 3 intervals for 4 attempts (no sleep after last attempt).
	if len(intervals) != 3 {
		t.Fatalf("expected 3 intervals, got %d", len(intervals))
	}

	// Check that intervals are increasing with exponential backoff.
	// The shared backoff library uses 50% jitter (randomization factor).
	// This means intervals vary within ±50% of the base value for robustness.
	// For InitialInterval=50ms with 50% jitter: [25ms, 75ms]
	// For 100ms base: [50ms, 150ms]
	// For 200ms base: [100ms, 300ms]
	// We verify intervals are within the jittered ranges.

	// First interval: base 50ms with 50% jitter = [25ms, 75ms], plus timing tolerance.
	minFirst := 20 * time.Millisecond // 25ms - 5ms tolerance
	maxFirst := 80 * time.Millisecond // 75ms + 5ms tolerance
	if intervals[0] < minFirst || intervals[0] > maxFirst {
		t.Errorf("expected first interval in range [%v, %v] with jitter, got %v", minFirst, maxFirst, intervals[0])
	}

	// Second interval: base 100ms with 50% jitter = [50ms, 150ms], plus timing tolerance.
	minSecond := 45 * time.Millisecond  // 50ms - 5ms tolerance
	maxSecond := 155 * time.Millisecond // 150ms + 5ms tolerance
	if intervals[1] < minSecond || intervals[1] > maxSecond {
		t.Errorf("expected second interval in range [%v, %v] with jitter, got %v", minSecond, maxSecond, intervals[1])
	}

	// Third interval: base 200ms (capped at MaxInterval) with 50% jitter = [100ms, 300ms], plus timing tolerance.
	minThird := 95 * time.Millisecond  // 100ms - 5ms tolerance
	maxThird := 305 * time.Millisecond // 300ms + 5ms tolerance
	if intervals[2] < minThird || intervals[2] > maxThird {
		t.Errorf("expected third interval in range [%v, %v] with jitter (capped), got %v", minThird, maxThird, intervals[2])
	}
}
