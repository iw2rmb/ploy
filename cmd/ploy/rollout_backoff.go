package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// RetryPolicy is a thin adapter that wraps backoff.Policy for rollout operations.
// Preserves the existing rollout API while delegating to the shared backoff package.
type RetryPolicy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxAttempts     int
}

// DefaultRetryPolicy returns sensible defaults for rollout operations.
// Matches existing rollout backoff behavior: 2s initial, 30s max, 2.0 multiplier, 10 attempts.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		InitialInterval: 2 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxAttempts:     10,
	}
}

// toBackoffPolicy converts a rollout RetryPolicy to a shared backoff.Policy.
// Maps rollout-specific fields to the shared package's policy structure.
func (p RetryPolicy) toBackoffPolicy() backoff.Policy {
	return backoff.Policy{
		InitialInterval: domaintypes.Duration(p.InitialInterval),
		MaxInterval:     domaintypes.Duration(p.MaxInterval),
		Multiplier:      p.Multiplier,
		MaxAttempts:     p.MaxAttempts,
		MaxElapsedTime:  0, // No time limit for rollout operations.
	}
}

// RetryWithBackoff executes a function with exponential backoff.
// Thin adapter around backoff.RunWithBackoff that preserves existing retry semantics.
// Returns nil if the function succeeds within maxAttempts, or the last error otherwise.
func RetryWithBackoff(ctx context.Context, policy RetryPolicy, fn func() error) error {
	// Convert to shared backoff policy and delegate to the shared helper.
	bp := policy.toBackoffPolicy()
	return backoff.RunWithBackoff(ctx, bp, slog.Default(), fn)
}

// PollWithBackoff polls a condition function with exponential backoff until it returns true or context expires.
// Thin adapter around shared backoff package that preserves rollout-specific logging and metrics.
// Logs each retry attempt with structured logging (poll_backoff_attempt, poll_backoff_exhausted).
// Records metrics via RolloutMetrics.RecordAttempt for observability.
func PollWithBackoff(ctx context.Context, policy RetryPolicy, logger *slog.Logger, metrics *RolloutMetrics, step string, condition func() (bool, error)) error {
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = NewRolloutMetrics()
	}

	// Convert to shared backoff policy.
	bp := policy.toBackoffPolicy()

	// Track attempts and errors for rollout-specific logging.
	attempt := 0
	lastErr := error(nil)

	// Wrap the condition function to integrate metrics recording and custom logging.
	wrappedCondition := func() (bool, error) {
		attempt++
		metrics.RecordAttempt(step)

		ok, err := condition()
		if err != nil {
			lastErr = err
			// Log attempt with error status using rollout-specific fields.
			logger.Debug("poll_backoff_attempt", "step", step, "attempt", attempt, "max_attempts", policy.MaxAttempts, "status", "error", "error", err.Error())
			return false, err
		}
		if ok {
			// Log success using rollout-specific fields.
			logger.Debug("poll_backoff_attempt", "step", step, "attempt", attempt, "max_attempts", policy.MaxAttempts, "status", "success")
			return true, nil
		}

		lastErr = fmt.Errorf("condition not met")
		// Log retry status using rollout-specific fields.
		logger.Debug("poll_backoff_attempt", "step", step, "attempt", attempt, "max_attempts", policy.MaxAttempts, "status", "retry")
		return false, fmt.Errorf("condition not met")
	}

	// Use the shared backoff helper with a custom logger that suppresses its default logs.
	// We handle logging ourselves in wrappedCondition to preserve exact rollout log format.
	// Discard logs from the shared helper by setting level higher than any error.
	quietLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.Level(1000)}))
	err := backoff.PollWithBackoff(ctx, bp, quietLogger, wrappedCondition)

	// Log exhaustion or cancellation using rollout-specific fields.
	if err != nil {
		switch {
		case ctx.Err() != nil && lastErr != nil:
			logger.Warn("poll_backoff_cancelled", "step", step, "attempt", attempt, "error", lastErr.Error())
		case lastErr != nil:
			logger.Error("poll_backoff_exhausted", "step", step, "max_attempts", policy.MaxAttempts, "error", lastErr.Error())
		default:
			logger.Error("poll_backoff_exhausted", "step", step, "max_attempts", policy.MaxAttempts)
		}
	}

	return err
}
