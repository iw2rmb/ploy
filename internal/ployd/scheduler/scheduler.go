package scheduler

import (
	"context"
	"sync"
	"time"
)

// Task defines a background job.
type Task interface {
	Name() string
	Interval() time.Duration
	Run(ctx context.Context) error
}

// Scheduler executes registered tasks at their intervals.
type Scheduler struct {
	mu      sync.Mutex
	tasks   []Task
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	group   sync.WaitGroup
}

// New constructs a scheduler instance.
func New() *Scheduler {
	return &Scheduler{}
}

// AddTask registers a task for execution.
func (s *Scheduler) AddTask(task Task) {
	if task == nil {
		return
	}
	s.mu.Lock()
	s.tasks = append(s.tasks, task)
	s.mu.Unlock()
}

// Start begins executing tasks.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	for _, task := range s.tasks {
		s.group.Add(1)
		t := task
		go s.runTask(t)
	}
	s.mu.Unlock()
	return nil
}

// Stop halts task execution.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	s.cancel = nil
	s.running = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.group.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) runTask(task Task) {
	defer s.group.Done()
	interval := task.Interval()
	if interval <= 0 {
		interval = time.Minute
	}
	ctx := s.taskContext()
	for {
		if ctx.Err() != nil {
			return
		}
		_ = task.Run(ctx)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Scheduler) taskContext() context.Context {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	return ctx
}
