package nodeagent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestConcurrency_LimitsJobExecution(t *testing.T) {
	t.Parallel()

	const concurrency = 2
	const totalJobs = 5

	cfg := newAgentConfig("http://localhost:8080", withConcurrency(concurrency))

	// Create controller with configured concurrency and typed JobID keys.
	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	// Track concurrent execution count.
	var currentConcurrency atomic.Int32
	var maxObservedConcurrency atomic.Int32
	var completedJobs atomic.Int32

	// jobStarted signals when a job has started execution.
	jobStarted := make(chan struct{}, totalJobs)
	// jobRelease signals jobs to complete (used to hold jobs in execution).
	jobRelease := make(chan struct{})

	// Start totalJobs goroutines that each acquire a slot and simulate work.
	var wg sync.WaitGroup
	for i := 0; i < totalJobs; i++ {
		wg.Add(1)
		go func(jobNum int) {
			defer wg.Done()

			ctx := context.Background()

			// Acquire a concurrency slot (blocks if at capacity).
			if err := rc.AcquireSlot(ctx); err != nil {
				t.Errorf("job %d: AcquireSlot failed: %v", jobNum, err)
				return
			}

			// Job is now executing - increment concurrency counter.
			current := currentConcurrency.Add(1)

			// Track max observed concurrency.
			for {
				maxObs := maxObservedConcurrency.Load()
				if current <= maxObs || maxObservedConcurrency.CompareAndSwap(maxObs, current) {
					break
				}
			}

			// Signal that this job has started.
			jobStarted <- struct{}{}

			// Wait for release signal (simulates job doing work).
			<-jobRelease

			// Job completing - decrement concurrency counter.
			currentConcurrency.Add(-1)
			completedJobs.Add(1)

			// Release the slot.
			rc.ReleaseSlot()
		}(i)
	}

	// Wait for exactly 'concurrency' jobs to start (others should be blocked).
	for i := 0; i < concurrency; i++ {
		select {
		case <-jobStarted:
			// Job started as expected.
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for job %d to start", i)
		}
	}

	// Give a brief moment for any additional jobs to (incorrectly) start.
	time.Sleep(100 * time.Millisecond)

	// Verify that exactly 'concurrency' jobs are running.
	current := currentConcurrency.Load()
	if current != int32(concurrency) {
		t.Errorf("expected %d concurrent jobs, got %d", concurrency, current)
	}

	// Close jobRelease to let all jobs complete.
	close(jobRelease)

	// Wait for remaining jobs to complete.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All jobs completed.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for jobs to complete")
	}

	// Verify all jobs completed.
	if completed := completedJobs.Load(); completed != totalJobs {
		t.Errorf("expected %d completed jobs, got %d", totalJobs, completed)
	}

	// Verify max concurrency never exceeded limit.
	maxObs := maxObservedConcurrency.Load()
	if maxObs > int32(concurrency) {
		t.Errorf("max observed concurrency %d exceeded limit %d", maxObs, concurrency)
	}
}

func TestConcurrency_DefaultsToOne(t *testing.T) {
	t.Parallel()

	cfg := newAgentConfig("http://localhost:8080", withConcurrency(0)) // Not set - should default to 1.

	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	// Track concurrent execution.
	var currentConcurrency atomic.Int32
	var maxObservedConcurrency atomic.Int32

	const totalJobs = 3
	jobStarted := make(chan struct{}, totalJobs)
	jobRelease := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < totalJobs; i++ {
		wg.Add(1)
		go func(jobNum int) {
			defer wg.Done()

			ctx := context.Background()
			if err := rc.AcquireSlot(ctx); err != nil {
				t.Errorf("job %d: AcquireSlot failed: %v", jobNum, err)
				return
			}

			current := currentConcurrency.Add(1)
			for {
				maxObs := maxObservedConcurrency.Load()
				if current <= maxObs || maxObservedConcurrency.CompareAndSwap(maxObs, current) {
					break
				}
			}

			jobStarted <- struct{}{}
			<-jobRelease

			currentConcurrency.Add(-1)
			rc.ReleaseSlot()
		}(i)
	}

	// Wait for first job to start.
	select {
	case <-jobStarted:
		// First job started.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first job to start")
	}

	// Give time for additional jobs to (incorrectly) start.
	time.Sleep(100 * time.Millisecond)

	// Verify only 1 job is running (default concurrency).
	current := currentConcurrency.Load()
	if current != 1 {
		t.Errorf("expected 1 concurrent job (default), got %d", current)
	}

	close(jobRelease)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All jobs completed.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for jobs to complete")
	}

	// Verify max concurrency was 1.
	if maxObs := maxObservedConcurrency.Load(); maxObs > 1 {
		t.Errorf("max observed concurrency %d exceeded default limit 1", maxObs)
	}
}

func TestConcurrency_ContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := newAgentConfig("http://localhost:8080", withConcurrency(1)) // Only 1 slot available.

	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	// Acquire the only slot.
	ctx := context.Background()
	if err := rc.AcquireSlot(ctx); err != nil {
		t.Fatalf("initial AcquireSlot failed: %v", err)
	}

	// Try to acquire with a cancellable context.
	cancelCtx, cancel := context.WithCancel(context.Background())

	// Start a goroutine that tries to acquire (will block).
	errCh := make(chan error, 1)
	go func() {
		errCh <- rc.AcquireSlot(cancelCtx)
	}()

	// Give the goroutine time to block on acquire.
	time.Sleep(50 * time.Millisecond)

	// Cancel the context.
	cancel()

	// Verify that AcquireSlot returns context.Canceled.
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AcquireSlot to return after cancel")
	}

	// Release the original slot.
	rc.ReleaseSlot()
}

func TestConcurrency_SlotReleasedOnJobCompletion(t *testing.T) {
	t.Parallel()

	cfg := newAgentConfig("http://localhost:8080", withConcurrency(1))

	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	// Acquire and release a slot multiple times.
	for i := 0; i < 5; i++ {
		ctx := context.Background()
		if err := rc.AcquireSlot(ctx); err != nil {
			t.Fatalf("iteration %d: AcquireSlot failed: %v", i, err)
		}
		rc.ReleaseSlot()
	}

	// All iterations should complete without blocking indefinitely.
	// If slots weren't being released properly, this would deadlock.
}
