package main

import (
	"context"
	"fmt"
	"time"
)

// RetryPolicy defines the configuration for exponential backoff retry logic.
type RetryPolicy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxAttempts     int
}

// DefaultRetryPolicy returns sensible defaults for rollout operations.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		InitialInterval: 2 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxAttempts:     10,
	}
}

// RetryWithBackoff executes a function with exponential backoff.
// It returns nil if the function succeeds within maxAttempts, or the last error otherwise.
func RetryWithBackoff(ctx context.Context, policy RetryPolicy, fn func() error) error {
	var lastErr error
	interval := policy.InitialInterval

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("context cancelled after %d attempts: %w", attempt, lastErr)
			}
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't sleep after the last attempt.
		if attempt == policy.MaxAttempts-1 {
			break
		}

		// Sleep with exponential backoff.
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return fmt.Errorf("context cancelled after %d attempts: %w", attempt+1, lastErr)
		}

		// Calculate next interval with multiplier, capped at MaxInterval.
		interval = time.Duration(float64(interval) * policy.Multiplier)
		if interval > policy.MaxInterval {
			interval = policy.MaxInterval
		}
	}

	return fmt.Errorf("exhausted %d attempts: %w", policy.MaxAttempts, lastErr)
}

// PollWithBackoff polls a condition function with exponential backoff until it returns true or context expires.
func PollWithBackoff(ctx context.Context, policy RetryPolicy, condition func() (bool, error)) error {
	return RetryWithBackoff(ctx, policy, func() error {
		ok, err := condition()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("condition not met")
		}
		return nil
	})
}
