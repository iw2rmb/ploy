package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/scheduler"
)

// TestStartTwice_AndStopNotRunning exercises Start when already running and
// Stop when not running to cover early-return paths.
func TestStartTwice_AndStopNotRunning(t *testing.T) {
	s := scheduler.New()

	// Stop when not running is a no-op.
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() when not running returned error: %v", err)
	}

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
	s.AddTask(&stubTask{
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
	case <-time.After(200 * time.Millisecond):
		t.Fatal("task did not run at least once")
	}
}

// TestStopContextDeadline ensures Stop respects the provided context deadline.
func TestStopContextDeadline(t *testing.T) {
	s := scheduler.New()
	s.AddTask(&stubTask{
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
	time.Sleep(10 * time.Millisecond)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer stopCancel()
	if err := s.Stop(stopCtx); err == nil {
		t.Fatal("expected Stop to return context error with deadline")
	}
	cancel()
	_ = s.Stop(context.Background())
}

func TestAddTaskNilIsNoop(t *testing.T) {
	s := scheduler.New()
	s.AddTask(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}
