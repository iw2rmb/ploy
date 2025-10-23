package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/ployd/scheduler"
)

func TestSchedulerRunsTasks(t *testing.T) {
	flag := make(chan struct{}, 1)
	s := scheduler.New()
	s.AddTask(&stubTask{
		name:     "cleanup",
		interval: 20 * time.Millisecond,
		run: func(context.Context) error {
			select {
			case flag <- struct{}{}:
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
	select {
	case <-flag:
	case <-time.After(200 * time.Millisecond):
		cancel()
		t.Fatal("task did not execute")
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

type stubTask struct {
	name     string
	interval time.Duration
	run      func(context.Context) error
}

func (s *stubTask) Name() string { return s.name }

func (s *stubTask) Interval() time.Duration { return s.interval }

func (s *stubTask) Run(ctx context.Context) error {
	return s.run(ctx)
}
