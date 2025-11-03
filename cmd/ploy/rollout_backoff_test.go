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
	err := PollWithBackoff(ctx, policy, func() (bool, error) {
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
	err := PollWithBackoff(ctx, policy, func() (bool, error) {
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
	err := PollWithBackoff(ctx, policy, func() (bool, error) {
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
	// Allow 30ms tolerance for timing variance.
	tolerance := 30 * time.Millisecond

	// First interval: ~50ms.
	if intervals[0] < 50*time.Millisecond-tolerance || intervals[0] > 50*time.Millisecond+tolerance {
		t.Errorf("expected first interval ~50ms, got %v", intervals[0])
	}

	// Second interval: ~100ms (50 * 2).
	if intervals[1] < 100*time.Millisecond-tolerance || intervals[1] > 100*time.Millisecond+tolerance {
		t.Errorf("expected second interval ~100ms, got %v", intervals[1])
	}

	// Third interval: ~200ms (100 * 2, capped at MaxInterval).
	if intervals[2] < 200*time.Millisecond-tolerance || intervals[2] > 200*time.Millisecond+tolerance {
		t.Errorf("expected third interval ~200ms (capped), got %v", intervals[2])
	}
}
