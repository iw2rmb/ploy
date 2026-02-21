package backoff

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRolloutPolicy verifies rollout policy matches existing rollout defaults.
func TestRolloutPolicy(t *testing.T) {
	t.Parallel()
	p := RolloutPolicy()

	if time.Duration(p.InitialInterval) != 2*time.Second {
		t.Errorf("InitialInterval = %v, want 2s", p.InitialInterval)
	}
	if time.Duration(p.MaxInterval) != 30*time.Second {
		t.Errorf("MaxInterval = %v, want 30s", p.MaxInterval)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", p.Multiplier)
	}
}

// TestHeartbeatPolicy verifies heartbeat policy matches nodeagent heartbeat defaults.
func TestHeartbeatPolicy(t *testing.T) {
	t.Parallel()
	p := HeartbeatPolicy()

	if time.Duration(p.InitialInterval) != 5*time.Second {
		t.Errorf("InitialInterval = %v, want 5s", p.InitialInterval)
	}
	if time.Duration(p.MaxInterval) != 5*time.Minute {
		t.Errorf("MaxInterval = %v, want 5m", p.MaxInterval)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", p.Multiplier)
	}
	if p.MaxElapsedTime != 0 {
		t.Errorf("MaxElapsedTime = %v, want 0 (infinite)", p.MaxElapsedTime)
	}
	if p.MaxAttempts != 0 {
		t.Errorf("MaxAttempts = %d, want 0 (infinite)", p.MaxAttempts)
	}
}

// TestClaimLoopPolicy verifies claim loop policy matches nodeagent claim loop defaults.
func TestClaimLoopPolicy(t *testing.T) {
	t.Parallel()
	p := ClaimLoopPolicy()

	if time.Duration(p.InitialInterval) != 250*time.Millisecond {
		t.Errorf("InitialInterval = %v, want 250ms", p.InitialInterval)
	}
	if time.Duration(p.MaxInterval) != 5*time.Second {
		t.Errorf("MaxInterval = %v, want 5s", p.MaxInterval)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", p.Multiplier)
	}
}

// TestGitLabMRPolicy verifies GitLab MR policy matches existing retry behavior.
func TestGitLabMRPolicy(t *testing.T) {
	t.Parallel()
	p := GitLabMRPolicy()

	// Initial delay: 1s.
	if time.Duration(p.InitialInterval) != 1*time.Second {
		t.Errorf("InitialInterval = %v, want 1s", p.InitialInterval)
	}
	// Max delay: 4s (1s * 2^2).
	if time.Duration(p.MaxInterval) != 4*time.Second {
		t.Errorf("MaxInterval = %v, want 4s", p.MaxInterval)
	}
	// Multiplier: 2.0 for exponential backoff.
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", p.Multiplier)
	}
	// No time limit for GitLab MR retries.
	if p.MaxElapsedTime != 0 {
		t.Errorf("MaxElapsedTime = %v, want 0 (no limit)", p.MaxElapsedTime)
	}
	// Max 4 attempts (initial + 3 retries).
	if p.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4", p.MaxAttempts)
	}
}

// TestNewExponentialBackoff verifies exponential backoff instance creation.
func TestNewExponentialBackoff(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(1 * time.Second),
		MaxInterval:     domaintypes.Duration(10 * time.Second),
		Multiplier:      2.0,
		MaxElapsedTime:  domaintypes.Duration(1 * time.Minute),
		MaxAttempts:     5,
	}

	eb := p.NewExponentialBackoff()

	if eb.InitialInterval != 1*time.Second {
		t.Errorf("InitialInterval = %v, want 1s", eb.InitialInterval)
	}
	if eb.MaxInterval != 10*time.Second {
		t.Errorf("MaxInterval = %v, want 10s", eb.MaxInterval)
	}
	if eb.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", eb.Multiplier)
	}
	// RandomizationFactor should be 0.5 (50% jitter).
	if eb.RandomizationFactor != 0.5 {
		t.Errorf("RandomizationFactor = %v, want 0.5", eb.RandomizationFactor)
	}
}

// TestRunWithBackoff_Success verifies immediate success without retries.
func TestRunWithBackoff_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(100 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(1 * time.Second),
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	calls := 0
	op := func() error {
		calls++
		return nil
	}

	err := RunWithBackoff(ctx, p, slog.Default(), op)
	if err != nil {
		t.Errorf("RunWithBackoff() = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("op called %d times, want 1", calls)
	}
}

// TestRunWithBackoff_RetryUntilSuccess verifies retries until operation succeeds.
func TestRunWithBackoff_RetryUntilSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     5,
	}

	calls := 0
	op := func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := RunWithBackoff(ctx, p, slog.Default(), op)
	if err != nil {
		t.Errorf("RunWithBackoff() = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("op called %d times, want 3", calls)
	}
}

// TestRunWithBackoff_ExhaustAttempts verifies max attempts limit is enforced.
func TestRunWithBackoff_ExhaustAttempts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	calls := 0
	op := func() error {
		calls++
		return errors.New("persistent error")
	}

	err := RunWithBackoff(ctx, p, slog.Default(), op)
	if err == nil {
		t.Error("RunWithBackoff() = nil, want error")
	}
	// MaxAttempts=3 means 3 attempts total.
	if calls != 3 {
		t.Errorf("op called %d times, want 3", calls)
	}
}

// TestRunWithBackoff_ContextCancellation verifies early exit on context cancellation.
func TestRunWithBackoff_ContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	p := Policy{
		InitialInterval: domaintypes.Duration(100 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(1 * time.Second),
		Multiplier:      2.0,
		MaxAttempts:     10,
	}

	calls := 0
	op := func() error {
		calls++
		if calls == 2 {
			cancel() // Cancel context on second call.
		}
		return errors.New("error")
	}

	err := RunWithBackoff(ctx, p, slog.Default(), op)
	if err == nil {
		t.Error("RunWithBackoff() = nil, want error")
	}
	// Should stop after cancellation (at most 2 calls, possibly 3 due to timing).
	if calls > 4 {
		t.Errorf("op called %d times, want <= 4 (context cancelled early)", calls)
	}
}

// TestPollWithBackoff_ConditionMet verifies immediate success when condition is true.
func TestPollWithBackoff_ConditionMet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(100 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(1 * time.Second),
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	calls := 0
	condition := func() (bool, error) {
		calls++
		return true, nil
	}

	err := PollWithBackoff(ctx, p, slog.Default(), condition)
	if err != nil {
		t.Errorf("PollWithBackoff() = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("condition called %d times, want 1", calls)
	}
}

// TestPollWithBackoff_ConditionEventuallyMet verifies retries until condition becomes true.
func TestPollWithBackoff_ConditionEventuallyMet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     5,
	}

	calls := 0
	condition := func() (bool, error) {
		calls++
		return calls >= 3, nil
	}

	err := PollWithBackoff(ctx, p, slog.Default(), condition)
	if err != nil {
		t.Errorf("PollWithBackoff() = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("condition called %d times, want 3", calls)
	}
}

// TestPollWithBackoff_ConditionError verifies error propagation from condition.
func TestPollWithBackoff_ConditionError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     3,
	}

	calls := 0
	condition := func() (bool, error) {
		calls++
		return false, errors.New("condition error")
	}

	err := PollWithBackoff(ctx, p, slog.Default(), condition)
	if err == nil {
		t.Error("PollWithBackoff() = nil, want error")
	}
	if calls != 3 {
		t.Errorf("condition called %d times, want 3", calls)
	}
}

// TestPollWithBackoff_ExhaustAttempts verifies max attempts limit for polling.
func TestPollWithBackoff_ExhaustAttempts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     4,
	}

	calls := 0
	condition := func() (bool, error) {
		calls++
		return false, nil // Never true.
	}

	err := PollWithBackoff(ctx, p, slog.Default(), condition)
	if err == nil {
		t.Error("PollWithBackoff() = nil, want error")
	}
	if calls != 4 {
		t.Errorf("condition called %d times, want 4", calls)
	}
}

// TestStatefulBackoff_ApplyAndReset verifies stateful backoff progression and reset.
func TestStatefulBackoff_ApplyAndReset(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(100 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(1 * time.Second),
		Multiplier:      2.0,
		MaxElapsedTime:  0, // Infinite.
		MaxAttempts:     0, // Infinite.
	}

	sb := NewStatefulBackoff(p)

	// Initial state: GetDuration returns initial interval.
	if d := time.Duration(sb.GetDuration()); d != 100*time.Millisecond {
		t.Errorf("GetDuration() = %v, want 100ms (initial)", d)
	}

	// Apply backoff: uses NextBackOff() which returns initial interval with jitter.
	// For InitialInterval=100ms with 50% jitter: [50ms, 150ms].
	d1 := sb.Apply()
	if time.Duration(d1) < 50*time.Millisecond || time.Duration(d1) > 150*time.Millisecond {
		t.Errorf("Apply() = %v, want in range [50ms, 150ms]", d1)
	}

	// Apply again: NextBackOff() doubles the base with jitter.
	// Expected base ~200ms with 50% jitter: [100ms, 300ms], capped at 1s.
	d2 := sb.Apply()
	if time.Duration(d2) < 100*time.Millisecond || time.Duration(d2) > 1*time.Second {
		t.Errorf("Apply() = %v, want in range [100ms, 1s]", d2)
	}

	// Apply again: should continue to grow (with jitter), capped at max 1s.
	d3 := sb.Apply()
	if time.Duration(d3) < 100*time.Millisecond || time.Duration(d3) > 1*time.Second {
		t.Errorf("Apply() = %v, want in range [100ms, 1s]", d3)
	}

	// Reset: should return to initial state.
	sb.Reset()
	if d := time.Duration(sb.GetDuration()); d != 100*time.Millisecond {
		t.Errorf("GetDuration() after Reset = %v, want 100ms", d)
	}

	// Apply after reset: should start from initial again with jitter.
	d4 := sb.Apply()
	if time.Duration(d4) < 50*time.Millisecond || time.Duration(d4) > 150*time.Millisecond {
		t.Errorf("Apply() after Reset = %v, want in range [50ms, 150ms]", d4)
	}
}

// TestStatefulBackoff_CapAtMaxInterval verifies backoff caps at max interval.
func TestStatefulBackoff_CapAtMaxInterval(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(50 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(200 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0, // Infinite.
		MaxAttempts:     0, // Infinite.
	}

	sb := NewStatefulBackoff(p)

	// Apply backoff multiple times; should eventually cap at 200ms.
	var lastDuration time.Duration
	for i := 0; i < 10; i++ {
		lastDuration = time.Duration(sb.Apply())
	}

	// After many applies, should be capped at max interval (200ms).
	// With 50% jitter on 200ms: [100ms, 300ms].
	// However, MaxInterval caps the base, so with jitter: [100ms, 300ms].
	if lastDuration > 300*time.Millisecond {
		t.Errorf("Apply() capped at %v, want <= 300ms (200ms base + 50%% jitter)", lastDuration)
	}
}

// TestStatefulBackoff_JitterBounds verifies jitter stays within bounds.
func TestStatefulBackoff_JitterBounds(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(1 * time.Second),
		MaxInterval:     domaintypes.Duration(10 * time.Second),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}

	sb := NewStatefulBackoff(p)

	// First apply: NextBackOff() returns initial interval with jitter.
	// For InitialInterval=1s with 50% jitter: [500ms, 1.5s].
	d1 := sb.Apply()
	if time.Duration(d1) < 500*time.Millisecond || time.Duration(d1) > 1500*time.Millisecond {
		t.Errorf("Apply() = %v, want in range [500ms, 1.5s]", d1)
	}

	// Second apply: NextBackOff() doubles the base with jitter.
	// Base 2s with 50% jitter: [1s, 3s].
	d2 := sb.Apply()
	if time.Duration(d2) < 1*time.Second || time.Duration(d2) > 3*time.Second {
		t.Errorf("Apply() = %v, want in range [1s, 3s]", d2)
	}

	// Third apply: NextBackOff() doubles again with jitter.
	// Base 4s with 50% jitter: [2s, 6s].
	d3 := sb.Apply()
	if time.Duration(d3) < 2*time.Second || time.Duration(d3) > 6*time.Second {
		t.Errorf("Apply() = %v, want in range [2s, 6s]", d3)
	}
}

// TestRunWithBackoff_NilLogger verifies nil logger defaults to slog.Default().
func TestRunWithBackoff_NilLogger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     2,
	}

	calls := 0
	op := func() error {
		calls++
		return nil
	}

	// Pass nil logger; should not panic.
	err := RunWithBackoff(ctx, p, nil, op)
	if err != nil {
		t.Errorf("RunWithBackoff() = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("op called %d times, want 1", calls)
	}
}

// TestPollWithBackoff_NilLogger verifies nil logger defaults to slog.Default().
func TestPollWithBackoff_NilLogger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     2,
	}

	calls := 0
	condition := func() (bool, error) {
		calls++
		return true, nil
	}

	// Pass nil logger; should not panic.
	err := PollWithBackoff(ctx, p, nil, condition)
	if err != nil {
		t.Errorf("PollWithBackoff() = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("condition called %d times, want 1", calls)
	}
}

// TestRunWithBackoff_ZeroMaxAttempts verifies infinite retries when MaxAttempts=0.
// We limit the test by cancelling context after a few attempts.
func TestRunWithBackoff_ZeroMaxAttempts(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(20 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     0, // Infinite attempts.
	}

	calls := 0
	op := func() error {
		calls++
		return errors.New("persistent error")
	}

	err := RunWithBackoff(ctx, p, slog.Default(), op)
	if err == nil {
		t.Error("RunWithBackoff() = nil, want error (context timeout)")
	}
	// Should retry multiple times until context timeout.
	if calls < 2 {
		t.Errorf("op called %d times, want >= 2", calls)
	}
}

// TestStatefulBackoff_GetDurationBeforeApply verifies GetDuration before any Apply.
func TestStatefulBackoff_GetDurationBeforeApply(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(500 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(5 * time.Second),
		Multiplier:      2.0,
	}

	sb := NewStatefulBackoff(p)

	// Before any Apply, GetDuration should return InitialInterval.
	if d := time.Duration(sb.GetDuration()); d != 500*time.Millisecond {
		t.Errorf("GetDuration() before Apply = %v, want 500ms", d)
	}
}
