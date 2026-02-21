// Package backoff provides centralized retry and backoff utilities using github.com/cenkalti/backoff/v5.
//
// This package offers helpers for exponential backoff with jitter, context cancellation,
// structured logging, and metrics hooks. It unifies retry logic across the codebase,
// replacing bespoke implementations in rollouts, nodeagent, and SSE clients.
package backoff

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/iw2rmb/ploy/internal/domain/types"
)

// Policy encapsulates exponential backoff configuration.
// Used to create backoff instances for retry operations.
type Policy struct {
	InitialInterval types.Duration
	MaxInterval     types.Duration
	Multiplier      float64
	MaxElapsedTime  types.Duration
	MaxAttempts     int
}

// RolloutPolicy returns a policy configured for rollout operations.
// Matches existing rollout backoff defaults: 2s initial, 30s max, 2.0 multiplier.
func RolloutPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(2 * time.Second),
		MaxInterval:     types.Duration(30 * time.Second),
		Multiplier:      2.0,
		MaxElapsedTime:  types.Duration(5 * time.Minute),
		MaxAttempts:     10,
	}
}

// HeartbeatPolicy returns a policy for nodeagent heartbeat backoff.
// Starts at 5s and caps at 5m to match existing heartbeat behavior.
func HeartbeatPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(5 * time.Second),
		MaxInterval:     types.Duration(5 * time.Minute),
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit, only 5xx errors trigger backoff.
		MaxAttempts:     0, // No attempt limit for heartbeat backoff.
	}
}

// ClaimLoopPolicy returns a policy for nodeagent claim loop polling.
// Starts at 250ms and caps at 5s to match existing claim loop behavior.
func ClaimLoopPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(250 * time.Millisecond),
		MaxInterval:     types.Duration(5 * time.Second),
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit for claim loop backoff.
		MaxAttempts:     0, // No attempt limit for claim loop backoff.
	}
}

// StatusUploaderPolicy returns a policy for nodeagent status upload retries.
// Starts at 100ms with 2x multiplier and 4 total attempts (initial + 3 retries).
// Matches existing status uploader retry behavior.
func StatusUploaderPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(100 * time.Millisecond),
		MaxInterval:     types.Duration(400 * time.Millisecond), // 100ms * 2^2 = 400ms (won't exceed this).
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit for status upload retries.
		MaxAttempts:     4, // Initial attempt + 3 retries.
	}
}

// CertificateBootstrapPolicy returns a policy for nodeagent certificate request retries.
// Starts at 1s with 2x multiplier and 5 total attempts (initial + 4 retries).
// Matches existing certificate bootstrap retry behavior (1s, 2s, 4s, 8s, 16s).
func CertificateBootstrapPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(1 * time.Second),
		MaxInterval:     types.Duration(16 * time.Second), // 1s * 2^4 = 16s max.
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit for certificate bootstrap retries.
		MaxAttempts:     5, // Initial attempt + 4 retries.
	}
}

// GitLabMRPolicy returns a policy for GitLab merge request API retries.
// Starts at 1s with 2x multiplier and 4 total attempts (initial + 3 retries).
// Matches existing GitLab MR retry behavior (1s, 2s, 4s).
func GitLabMRPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(1 * time.Second),
		MaxInterval:     types.Duration(4 * time.Second), // 1s * 2^2 = 4s max.
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit for GitLab MR retries.
		MaxAttempts:     4, // Initial attempt + 3 retries.
	}
}

// SSEStreamPolicy returns a policy for SSE stream reconnect backoff.
// Starts at 250ms with 2x multiplier, matching existing SSE default.
// Uses unlimited retries (-1) unless caller configures MaxRetries.
// This policy provides a base configuration; callers override MaxAttempts via Client.MaxRetries.
func SSEStreamPolicy() Policy {
	return Policy{
		InitialInterval: types.Duration(250 * time.Millisecond),
		MaxInterval:     types.Duration(30 * time.Second), // Cap reconnect delay at 30s.
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit; rely on MaxRetries or context cancellation.
		MaxAttempts:     0, // Unlimited by default; use Client.MaxRetries to cap.
	}
}

// NewExponentialBackoff creates a backoff.ExponentialBackOff from the policy.
// Configures initial interval, max interval, multiplier, randomization factor (jitter).
// Callers use this with backoff.Retry and options like WithMaxTries, WithMaxElapsedTime.
func (p Policy) NewExponentialBackoff() *backoff.ExponentialBackOff {
	eb := backoff.NewExponentialBackOff()
	eb.InitialInterval = time.Duration(p.InitialInterval)
	eb.MaxInterval = time.Duration(p.MaxInterval)
	eb.Multiplier = p.Multiplier
	eb.RandomizationFactor = 0.5 // 50% jitter for robustness.
	return eb
}

// retryLoop is the shared retry-loop builder used by RunWithBackoff and PollWithBackoff.
func retryLoop(ctx context.Context, policy Policy, logger *slog.Logger, notify func(err error, d time.Duration, attempt int), op func() (struct{}, error), exhausted func(attempt int, err error)) error {
	eb := policy.NewExponentialBackoff()

	opts := []backoff.RetryOption{
		backoff.WithBackOff(eb),
	}
	if policy.MaxAttempts > 0 {
		opts = append(opts, backoff.WithMaxTries(uint(policy.MaxAttempts)))
	}
	if policy.MaxElapsedTime > 0 {
		opts = append(opts, backoff.WithMaxElapsedTime(time.Duration(policy.MaxElapsedTime)))
	}

	attempt := 0
	opts = append(opts, backoff.WithNotify(func(err error, d time.Duration) {
		notify(err, d, attempt)
	}))

	wrappedOp := func() (struct{}, error) {
		attempt++
		return op()
	}

	_, err := backoff.Retry(ctx, wrappedOp, opts...)
	if err != nil {
		exhausted(attempt, err)
	}
	return err
}

// RunWithBackoff executes an operation with exponential backoff and retries.
// Uses the configured policy to determine intervals, max attempts, and jitter.
// Logs each retry attempt with structured fields (attempt, backoff_duration).
// Honors context cancellation and returns early if context is done.
// Returns nil if the operation succeeds, or the last error if max attempts exhausted.
func RunWithBackoff(ctx context.Context, policy Policy, logger *slog.Logger, op func() error) error {
	if logger == nil {
		logger = slog.Default()
	}

	return retryLoop(ctx, policy, logger,
		func(err error, d time.Duration, attempt int) {
			logger.Debug("backoff_attempt", "attempt", attempt, "next_backoff", d, "error", err.Error())
		},
		func() (struct{}, error) {
			if err := op(); err != nil {
				return struct{}{}, err
			}
			return struct{}{}, nil
		},
		func(attempt int, err error) {
			logger.Error("backoff_exhausted", "attempt", attempt, "error", err.Error())
		},
	)
}

// PollWithBackoff polls a condition function with exponential backoff until it returns true.
// Similar to RunWithBackoff but designed for polling: retries when condition returns (false, nil).
// Logs each poll attempt with structured fields (attempt, status, backoff_duration).
// Returns nil if condition succeeds, or error if max attempts exhausted or operation fails.
func PollWithBackoff(ctx context.Context, policy Policy, logger *slog.Logger, condition func() (bool, error)) error {
	if logger == nil {
		logger = slog.Default()
	}

	var lastStatus string
	var lastErr error

	return retryLoop(ctx, policy, logger,
		func(err error, d time.Duration, attempt int) {
			if lastErr != nil {
				logger.Debug("poll_backoff_attempt", "attempt", attempt, "next_backoff", d, "status", lastStatus, "error", lastErr.Error())
			} else {
				logger.Debug("poll_backoff_attempt", "attempt", attempt, "next_backoff", d, "status", lastStatus)
			}
		},
		func() (struct{}, error) {
			ok, err := condition()
			if err != nil {
				lastStatus = "error"
				lastErr = err
				return struct{}{}, err
			}
			if ok {
				return struct{}{}, nil
			}
			lastStatus = "retry"
			lastErr = nil
			return struct{}{}, fmt.Errorf("condition not met")
		},
		func(attempt int, err error) {
			if lastErr != nil {
				logger.Error("poll_backoff_exhausted", "attempt", attempt, "status", lastStatus, "error", lastErr.Error())
			} else {
				logger.Error("poll_backoff_exhausted", "attempt", attempt, "status", lastStatus)
			}
		},
	)
}

// StatefulBackoff manages exponential backoff state for scenarios where backoff
// needs to persist across events (e.g., heartbeat failures, claim loop polling).
// Callers trigger backoff via Apply() on errors and reset via Reset() on success.
// GetDuration() returns the current or next backoff interval.
type StatefulBackoff struct {
	eb      *backoff.ExponentialBackOff
	current types.Duration
	started bool
}

// NewStatefulBackoff creates a StatefulBackoff from a policy.
// Useful for long-running loops that maintain backoff state across iterations.
func NewStatefulBackoff(policy Policy) *StatefulBackoff {
	return &StatefulBackoff{
		eb:      policy.NewExponentialBackoff(),
		current: 0,
		started: false,
	}
}

// Apply triggers backoff, advancing to the next interval.
// Should be called on retry-triggering events (errors, no work available).
// Returns the new backoff duration.
func (s *StatefulBackoff) Apply() types.Duration {
	// Always get the next backoff duration (this advances the internal state).
	next := s.eb.NextBackOff()
	if next == backoff.Stop {
		// Max elapsed time reached or stopped; keep current or use max interval.
		if s.current > 0 {
			return s.current
		}
		return types.Duration(s.eb.MaxInterval)
	}
	s.current = types.Duration(next)
	s.started = true
	return s.current
}

// Reset clears backoff state, returning to the initial interval.
// Should be called on successful operations to reset exponential growth.
func (s *StatefulBackoff) Reset() {
	s.eb.Reset()
	s.current = 0
	s.started = false
}

// GetDuration returns the current backoff duration.
// If backoff has not been applied, returns the initial interval.
func (s *StatefulBackoff) GetDuration() types.Duration {
	if !s.started {
		return types.Duration(s.eb.InitialInterval)
	}
	return s.current
}

// Permanent wraps the given error in a *PermanentError to prevent retries.
// Use this to signal that an error should not be retried (e.g., validation errors, 4xx HTTP status).
// The underlying backoff.Retry will stop immediately when encountering a permanent error.
func Permanent(err error) error {
	return backoff.Permanent(err)
}
