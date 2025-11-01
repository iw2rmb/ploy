package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/scheduler"
)

// TestStartTwice_AndStopNotRunning exercises Start when already running and
// Stop when not running to cover early-return paths.
func TestStartTwice_AndStopNotRunning(t *testing.T) {
	s := scheduler.New()

	// Stop when not running is a no-op.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() when not running returned error: %v", err)
	}

	// Start the scheduler.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Starting again should be a no-op and not error.
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() when already running returned error: %v", err)
	}

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
}

// TestZeroIntervalTaskRunsOnce verifies that a task with zero interval (defaulted
// to 1 minute) still executes at least once at scheduler start.
func TestZeroIntervalTaskRunsOnce(t *testing.T) {
	ran := make(chan struct{}, 1)
	s := scheduler.New()
	s.AddTask(&stubTask2{
		name:     "zero",
		interval: 0,
		run: func(context.Context) error {
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	select {
	case <-ran:
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("task did not run at least once")
	}
}

// TestStopContextDeadline ensures Stop respects the provided context deadline
// and returns promptly with context error when waiting for tasks would block.
func TestStopContextDeadline(t *testing.T) {
	s := scheduler.New()
	// Task that sleeps longer than our Stop context deadline.
	s.AddTask(&stubTask2{
		name:     "slow",
		interval: time.Hour,
		run: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
				return nil
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// Give it a moment to start goroutine.
	time.Sleep(10 * time.Millisecond)
	// Request stop with a short deadline. The scheduler should return ctx.Err().
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer stopCancel()
	if err := s.Stop(stopCtx); err == nil {
		t.Fatal("expected Stop to return context error with deadline")
	}
	cancel()
	// Ensure clean shutdown to avoid leaking goroutines in the test process.
	_ = s.Stop(context.Background())
}

func TestAddTaskNilIsNoop(t *testing.T) {
	s := scheduler.New()
	// Should not panic or change internal state.
	s.AddTask(nil)
	// Start/stop with no tasks should work.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

type stubTask2 struct {
	name     string
	interval time.Duration
	run      func(context.Context) error
}

func (s *stubTask2) Name() string                  { return s.name }
func (s *stubTask2) Interval() time.Duration       { return s.interval }
func (s *stubTask2) Run(ctx context.Context) error { return s.run(ctx) }
