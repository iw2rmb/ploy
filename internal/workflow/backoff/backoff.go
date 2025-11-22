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
)

// Policy encapsulates exponential backoff configuration.
// Used to create backoff instances for retry operations.
type Policy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxElapsedTime  time.Duration
	MaxAttempts     int
}

// DefaultPolicy returns a sensible default policy for general retry operations.
// Initial interval: 2s, max interval: 30s, multiplier: 2.0, max elapsed: 5m, max attempts: 10.
func DefaultPolicy() Policy {
	return Policy{
		InitialInterval: 2 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxElapsedTime:  5 * time.Minute,
		MaxAttempts:     10,
	}
}

// RolloutPolicy returns a policy configured for rollout operations.
// Matches existing rollout backoff defaults: 2s initial, 30s max, 2.0 multiplier.
func RolloutPolicy() Policy {
	return Policy{
		InitialInterval: 2 * time.Second,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxElapsedTime:  5 * time.Minute,
		MaxAttempts:     10,
	}
}

// HeartbeatPolicy returns a policy for nodeagent heartbeat backoff.
// Starts at 5s and caps at 5m to match existing heartbeat behavior.
func HeartbeatPolicy() Policy {
	return Policy{
		InitialInterval: 5 * time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit, only 5xx errors trigger backoff.
		MaxAttempts:     0, // No attempt limit for heartbeat backoff.
	}
}

// ClaimLoopPolicy returns a policy for nodeagent claim loop polling.
// Starts at 250ms and caps at 5s to match existing claim loop behavior.
func ClaimLoopPolicy() Policy {
	return Policy{
		InitialInterval: 250 * time.Millisecond,
		MaxInterval:     5 * time.Second,
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
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     400 * time.Millisecond, // 100ms * 2^2 = 400ms (won't exceed this).
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
		InitialInterval: 1 * time.Second,
		MaxInterval:     16 * time.Second, // 1s * 2^4 = 16s max.
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
		InitialInterval: 1 * time.Second,
		MaxInterval:     4 * time.Second, // 1s * 2^2 = 4s max.
		Multiplier:      2.0,
		MaxElapsedTime:  0, // No time limit for GitLab MR retries.
		MaxAttempts:     4, // Initial attempt + 3 retries.
	}
}

// NewExponentialBackoff creates a backoff.ExponentialBackOff from the policy.
// Configures initial interval, max interval, multiplier, randomization factor (jitter).
// Callers use this with backoff.Retry and options like WithMaxTries, WithMaxElapsedTime.
func (p Policy) NewExponentialBackoff() *backoff.ExponentialBackOff {
	eb := backoff.NewExponentialBackOff()
	eb.InitialInterval = p.InitialInterval
	eb.MaxInterval = p.MaxInterval
	eb.Multiplier = p.Multiplier
	eb.RandomizationFactor = 0.5 // 50% jitter for robustness.
	return eb
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

	eb := policy.NewExponentialBackoff()

	// Build retry options.
	opts := []backoff.RetryOption{
		backoff.WithBackOff(eb),
	}
	if policy.MaxAttempts > 0 {
		opts = append(opts, backoff.WithMaxTries(uint(policy.MaxAttempts)))
	}
	if policy.MaxElapsedTime > 0 {
		opts = append(opts, backoff.WithMaxElapsedTime(policy.MaxElapsedTime))
	}

	// Add notify callback for logging.
	attempt := 0
	opts = append(opts, backoff.WithNotify(func(err error, d time.Duration) {
		logger.Debug("backoff_attempt", "attempt", attempt, "next_backoff", d, "error", err.Error())
	}))

	// Create operation that tracks attempts and logs success.
	operation := func() (struct{}, error) {
		attempt++
		err := op()
		if err != nil {
			return struct{}{}, err
		}
		logger.Debug("backoff_success", "attempt", attempt)
		return struct{}{}, nil
	}

	_, err := backoff.Retry(ctx, operation, opts...)
	if err != nil {
		logger.Error("backoff_exhausted", "attempt", attempt, "error", err.Error())
	}
	return err
}

// PollWithBackoff polls a condition function with exponential backoff until it returns true.
// Similar to RunWithBackoff but designed for polling: retries when condition returns (false, nil).
// Logs each poll attempt with structured fields (attempt, status, backoff_duration).
// Returns nil if condition succeeds, or error if max attempts exhausted or operation fails.
func PollWithBackoff(ctx context.Context, policy Policy, logger *slog.Logger, condition func() (bool, error)) error {
	if logger == nil {
		logger = slog.Default()
	}

	eb := policy.NewExponentialBackoff()

	// Build retry options.
	opts := []backoff.RetryOption{
		backoff.WithBackOff(eb),
	}
	if policy.MaxAttempts > 0 {
		opts = append(opts, backoff.WithMaxTries(uint(policy.MaxAttempts)))
	}
	if policy.MaxElapsedTime > 0 {
		opts = append(opts, backoff.WithMaxElapsedTime(policy.MaxElapsedTime))
	}

	// Track attempts for logging.
	attempt := 0
	lastStatus := ""
	lastErr := error(nil)

	// Add notify callback for logging retries.
	opts = append(opts, backoff.WithNotify(func(err error, d time.Duration) {
		if lastErr != nil {
			logger.Debug("poll_backoff_attempt", "attempt", attempt, "next_backoff", d, "status", lastStatus, "error", lastErr.Error())
		} else {
			logger.Debug("poll_backoff_attempt", "attempt", attempt, "next_backoff", d, "status", lastStatus)
		}
	}))

	// Create operation that checks condition and tracks status.
	operation := func() (struct{}, error) {
		attempt++
		ok, err := condition()
		if err != nil {
			lastStatus = "error"
			lastErr = err
			return struct{}{}, err
		}
		if ok {
			logger.Debug("poll_backoff_attempt", "attempt", attempt, "status", "success")
			return struct{}{}, nil
		}
		lastStatus = "retry"
		lastErr = nil
		return struct{}{}, fmt.Errorf("condition not met")
	}

	_, err := backoff.Retry(ctx, operation, opts...)
	if err != nil {
		if lastErr != nil {
			logger.Error("poll_backoff_exhausted", "attempt", attempt, "status", lastStatus, "error", lastErr.Error())
		} else {
			logger.Error("poll_backoff_exhausted", "attempt", attempt, "status", lastStatus)
		}
	}
	return err
}

// StatefulBackoff manages exponential backoff state for scenarios where backoff
// needs to persist across events (e.g., heartbeat failures, claim loop polling).
// Callers trigger backoff via Apply() on errors and reset via Reset() on success.
// GetDuration() returns the current or next backoff interval.
type StatefulBackoff struct {
	eb      *backoff.ExponentialBackOff
	current time.Duration
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
func (s *StatefulBackoff) Apply() time.Duration {
	// Always get the next backoff duration (this advances the internal state).
	next := s.eb.NextBackOff()
	if next == backoff.Stop {
		// Max elapsed time reached or stopped; keep current or use max interval.
		if s.current > 0 {
			return s.current
		}
		return s.eb.MaxInterval
	}
	s.current = next
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
func (s *StatefulBackoff) GetDuration() time.Duration {
	if !s.started {
		return s.eb.InitialInterval
	}
	return s.current
}

// Permanent wraps the given error in a *PermanentError to prevent retries.
// Use this to signal that an error should not be retried (e.g., validation errors, 4xx HTTP status).
// The underlying backoff.Retry will stop immediately when encountering a permanent error.
func Permanent(err error) error {
	return backoff.Permanent(err)
}
