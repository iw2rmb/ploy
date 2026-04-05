package backoff

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestPolicies verifies all named policy constructors return expected field values.
func TestPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		policy          Policy
		wantInitial     time.Duration
		wantMax         time.Duration
		wantMultiplier  float64
		wantMaxElapsed  time.Duration
		wantMaxAttempts int
	}{
		{
			name:           "rollout",
			policy:         RolloutPolicy(),
			wantInitial:    2 * time.Second,
			wantMax:        30 * time.Second,
			wantMultiplier: 2.0,
			wantMaxElapsed: 5 * time.Minute,
			wantMaxAttempts: 10,
		},
		{
			name:           "heartbeat",
			policy:         HeartbeatPolicy(),
			wantInitial:    5 * time.Second,
			wantMax:        5 * time.Minute,
			wantMultiplier: 2.0,
		},
		{
			name:           "claim loop",
			policy:         ClaimLoopPolicy(),
			wantInitial:    250 * time.Millisecond,
			wantMax:        5 * time.Second,
			wantMultiplier: 2.0,
		},
		{
			name:            "gitlab MR",
			policy:          GitLabMRPolicy(),
			wantInitial:     1 * time.Second,
			wantMax:         4 * time.Second,
			wantMultiplier:  2.0,
			wantMaxAttempts: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := tt.policy
			if got := time.Duration(p.InitialInterval); got != tt.wantInitial {
				t.Errorf("InitialInterval = %v, want %v", got, tt.wantInitial)
			}
			if got := time.Duration(p.MaxInterval); got != tt.wantMax {
				t.Errorf("MaxInterval = %v, want %v", got, tt.wantMax)
			}
			if p.Multiplier != tt.wantMultiplier {
				t.Errorf("Multiplier = %v, want %v", p.Multiplier, tt.wantMultiplier)
			}
			if got := time.Duration(p.MaxElapsedTime); got != tt.wantMaxElapsed {
				t.Errorf("MaxElapsedTime = %v, want %v", got, tt.wantMaxElapsed)
			}
			if p.MaxAttempts != tt.wantMaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", p.MaxAttempts, tt.wantMaxAttempts)
			}
		})
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
	if eb.RandomizationFactor != 0.5 {
		t.Errorf("RandomizationFactor = %v, want 0.5", eb.RandomizationFactor)
	}
}

func fastPolicy(maxAttempts int) Policy {
	return Policy{
		InitialInterval: domaintypes.Duration(10 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxAttempts:     maxAttempts,
	}
}

// TestRunWithBackoff verifies retry behavior for RunWithBackoff across scenarios.
func TestRunWithBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    Policy
		logger    *slog.Logger
		op        func(calls *int) error
		ctx       func() (context.Context, context.CancelFunc)
		wantErr   bool
		wantCalls func(calls int) bool
	}{
		{
			name:   "immediate success",
			policy: fastPolicy(3),
			op:     func(_ *int) error { return nil },
			wantCalls: func(c int) bool { return c == 1 },
		},
		{
			name:   "retry until success",
			policy: fastPolicy(5),
			op: func(calls *int) error {
				if *calls < 3 {
					return errors.New("temporary error")
				}
				return nil
			},
			wantCalls: func(c int) bool { return c == 3 },
		},
		{
			name:   "exhaust attempts",
			policy: fastPolicy(3),
			op:     func(_ *int) error { return errors.New("persistent error") },
			wantErr:   true,
			wantCalls: func(c int) bool { return c == 3 },
		},
		{
			name:   "context cancellation",
			policy: fastPolicy(10),
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			op: func(calls *int) error {
				if *calls == 2 {
					// Cancel happens via ctx setup below.
				}
				return errors.New("error")
			},
			wantErr:   true,
			wantCalls: func(c int) bool { return c <= 4 },
		},
		{
			name:   "nil logger does not panic",
			policy: fastPolicy(2),
			logger: nil,
			op:     func(_ *int) error { return nil },
			wantCalls: func(c int) bool { return c == 1 },
		},
		{
			name: "zero max attempts retries until context timeout",
			policy: Policy{
				InitialInterval: domaintypes.Duration(10 * time.Millisecond),
				MaxInterval:     domaintypes.Duration(20 * time.Millisecond),
				Multiplier:      2.0,
				MaxAttempts:     0,
			},
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 100*time.Millisecond)
			},
			op:        func(_ *int) error { return errors.New("persistent error") },
			wantErr:   true,
			wantCalls: func(c int) bool { return c >= 2 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.ctx != nil {
				ctx, cancel = tt.ctx()
				defer cancel()
			}

			calls := 0
			logger := tt.logger
			if logger == nil && tt.name != "nil logger does not panic" {
				logger = slog.Default()
			}

			// For context cancellation test, cancel on second call.
			op := func() error {
				calls++
				if tt.name == "context cancellation" && calls == 2 && cancel != nil {
					cancel()
				}
				return tt.op(&calls)
			}

			err := RunWithBackoff(ctx, tt.policy, logger, op)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunWithBackoff() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantCalls(calls) {
				t.Errorf("op called %d times, unexpected", calls)
			}
		})
	}
}

// TestPollWithBackoff verifies retry behavior for PollWithBackoff across scenarios.
func TestPollWithBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    Policy
		logger    *slog.Logger
		condition func(calls *int) (bool, error)
		wantErr   bool
		wantCalls int
	}{
		{
			name:      "condition met immediately",
			policy:    fastPolicy(3),
			condition: func(_ *int) (bool, error) { return true, nil },
			wantCalls: 1,
		},
		{
			name:   "condition eventually met",
			policy: fastPolicy(5),
			condition: func(calls *int) (bool, error) {
				return *calls >= 3, nil
			},
			wantCalls: 3,
		},
		{
			name:      "condition error propagated",
			policy:    fastPolicy(3),
			condition: func(_ *int) (bool, error) { return false, errors.New("condition error") },
			wantErr:   true,
			wantCalls: 3,
		},
		{
			name:      "exhaust attempts (never true)",
			policy:    fastPolicy(4),
			condition: func(_ *int) (bool, error) { return false, nil },
			wantErr:   true,
			wantCalls: 4,
		},
		{
			name:      "nil logger does not panic",
			policy:    fastPolicy(2),
			logger:    nil,
			condition: func(_ *int) (bool, error) { return true, nil },
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			calls := 0

			logger := tt.logger
			if logger == nil && tt.name != "nil logger does not panic" {
				logger = slog.Default()
			}

			condition := func() (bool, error) {
				calls++
				return tt.condition(&calls)
			}

			err := PollWithBackoff(ctx, tt.policy, logger, condition)
			if (err != nil) != tt.wantErr {
				t.Errorf("PollWithBackoff() error = %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("condition called %d times, want %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestStatefulBackoff_ApplyAndReset verifies stateful backoff progression and reset.
func TestStatefulBackoff_ApplyAndReset(t *testing.T) {
	t.Parallel()
	p := Policy{
		InitialInterval: domaintypes.Duration(100 * time.Millisecond),
		MaxInterval:     domaintypes.Duration(1 * time.Second),
		Multiplier:      2.0,
	}

	sb := NewStatefulBackoff(p)

	// Initial state.
	if d := time.Duration(sb.GetDuration()); d != 100*time.Millisecond {
		t.Errorf("GetDuration() = %v, want 100ms (initial)", d)
	}

	// Apply: initial interval with 50% jitter → [50ms, 150ms].
	d1 := sb.Apply()
	if time.Duration(d1) < 50*time.Millisecond || time.Duration(d1) > 150*time.Millisecond {
		t.Errorf("Apply() = %v, want in range [50ms, 150ms]", d1)
	}

	// Apply again: doubled base with jitter, capped at 1s.
	d2 := sb.Apply()
	if time.Duration(d2) < 100*time.Millisecond || time.Duration(d2) > 1*time.Second {
		t.Errorf("Apply() = %v, want in range [100ms, 1s]", d2)
	}

	// Reset and verify return to initial.
	sb.Reset()
	if d := time.Duration(sb.GetDuration()); d != 100*time.Millisecond {
		t.Errorf("GetDuration() after Reset = %v, want 100ms", d)
	}

	d4 := sb.Apply()
	if time.Duration(d4) < 50*time.Millisecond || time.Duration(d4) > 150*time.Millisecond {
		t.Errorf("Apply() after Reset = %v, want in range [50ms, 150ms]", d4)
	}
}

// TestStatefulBackoff_Behaviors tests cap-at-max, jitter bounds, and pre-apply duration.
func TestStatefulBackoff_Behaviors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		policy  Policy
		applies int
		check   func(t *testing.T, sb *StatefulBackoff)
	}{
		{
			name: "caps at max interval",
			policy: Policy{
				InitialInterval: domaintypes.Duration(50 * time.Millisecond),
				MaxInterval:     domaintypes.Duration(200 * time.Millisecond),
				Multiplier:      2.0,
			},
			applies: 10,
			check: func(t *testing.T, sb *StatefulBackoff) {
				// After many applies, should be capped. With 50% jitter on 200ms: ≤300ms.
				d := time.Duration(sb.Apply())
				if d > 300*time.Millisecond {
					t.Errorf("Apply() = %v, want ≤ 300ms (200ms base + 50%% jitter)", d)
				}
			},
		},
		{
			name: "jitter stays within bounds",
			policy: Policy{
				InitialInterval: domaintypes.Duration(1 * time.Second),
				MaxInterval:     domaintypes.Duration(10 * time.Second),
				Multiplier:      2.0,
			},
			applies: 0,
			check: func(t *testing.T, sb *StatefulBackoff) {
				// First: 1s base → [500ms, 1.5s].
				d1 := time.Duration(sb.Apply())
				if d1 < 500*time.Millisecond || d1 > 1500*time.Millisecond {
					t.Errorf("Apply()[0] = %v, want [500ms, 1.5s]", d1)
				}
				// Second: 2s base → [1s, 3s].
				d2 := time.Duration(sb.Apply())
				if d2 < 1*time.Second || d2 > 3*time.Second {
					t.Errorf("Apply()[1] = %v, want [1s, 3s]", d2)
				}
				// Third: 4s base → [2s, 6s].
				d3 := time.Duration(sb.Apply())
				if d3 < 2*time.Second || d3 > 6*time.Second {
					t.Errorf("Apply()[2] = %v, want [2s, 6s]", d3)
				}
			},
		},
		{
			name: "GetDuration before Apply returns initial interval",
			policy: Policy{
				InitialInterval: domaintypes.Duration(500 * time.Millisecond),
				MaxInterval:     domaintypes.Duration(5 * time.Second),
				Multiplier:      2.0,
			},
			applies: 0,
			check: func(t *testing.T, sb *StatefulBackoff) {
				if d := time.Duration(sb.GetDuration()); d != 500*time.Millisecond {
					t.Errorf("GetDuration() = %v, want 500ms", d)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sb := NewStatefulBackoff(tt.policy)
			for i := 0; i < tt.applies; i++ {
				sb.Apply()
			}
			tt.check(t, sb)
		})
	}
}
