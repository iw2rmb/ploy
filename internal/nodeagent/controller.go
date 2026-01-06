package nodeagent

import (
	"context"
	"fmt"
	"sync"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id (not run_id) to support multiple jobs per run.
//
// Concurrency is enforced via the jobSem semaphore: callers must acquire a slot
// via AcquireSlot before claiming/starting work. On StartRun success, the
// controller releases the slot when the job completes. This prevents the node
// from claiming more jobs than it can execute concurrently.
type runController struct {
	mu sync.Mutex

	cfg Config

	// jobs tracks active jobs by typed JobID key.
	// Using types.JobID as map key avoids stringly-typed lookups and provides
	// compile-time safety against mismatched ID types.
	jobs map[types.JobID]*jobContext

	// jobSem is a counting semaphore that limits concurrent job execution.
	// The capacity is set to Config.Concurrency (minimum 1).
	// AcquireSlot sends to this channel; ReleaseSlot receives from it.
	jobSem chan struct{}
}

type jobContext struct {
	// runID is the typed run identifier for this job's parent run.
	runID types.RunID
	// jobID is the typed job identifier, matching the map key in runController.jobs.
	jobID types.JobID
	// cancel terminates execution of this job when called.
	cancel context.CancelFunc
}

// initJobSem initializes the concurrency semaphore if not already initialized.
// Called lazily on first AcquireSlot to allow Config.Concurrency to be set.
// Thread-safe via the controller's mutex.
func (r *runController) initJobSem() {
	if r.jobSem != nil {
		return
	}
	// Use configured concurrency; minimum of 1 (already enforced in LoadConfig,
	// but we guard here defensively for direct struct construction in tests).
	capacity := r.cfg.Concurrency
	if capacity < 1 {
		capacity = 1
	}
	r.jobSem = make(chan struct{}, capacity)
}

// AcquireSlot blocks until a concurrency slot is available or the context
// is canceled. Returns nil when a slot is acquired, or ctx.Err() if the
// context was canceled while waiting.
//
// The semaphore ensures the node does not claim more work than it can execute
// concurrently. The slot must be released via ReleaseSlot when the job completes.
func (r *runController) AcquireSlot(ctx context.Context) error {
	r.mu.Lock()
	r.initJobSem()
	sem := r.jobSem
	r.mu.Unlock()

	// Block until we can acquire a slot or context is canceled.
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot frees a previously acquired concurrency slot.
// Must be called exactly once for each successful AcquireSlot call.
func (r *runController) ReleaseSlot() {
	r.mu.Lock()
	sem := r.jobSem
	r.mu.Unlock()

	if sem == nil {
		// Should not happen in normal operation, but guard defensively.
		return
	}
	<-sem
}

// StartRun accepts a run start request and initiates execution.
// Tracks by job_id to allow multiple jobs within the same run_id (e.g. pre-gate, mod-0).
// The jobs map uses typed types.JobID keys to prevent stringly-typed ID confusion.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Use typed JobID directly as map key — no intermediate string conversion needed.
	if _, exists := r.jobs[req.JobID]; exists {
		return fmt.Errorf("job %s already exists", req.JobID)
	}

	// Create a cancellable context for this job, derived from caller.
	runCtx, cancel := context.WithCancel(ctx)
	r.jobs[req.JobID] = &jobContext{
		runID:  req.RunID,
		jobID:  req.JobID,
		cancel: cancel,
	}

	// Execute the run/job in a goroutine.
	go r.executeRun(runCtx, req)

	return nil
}

// StopRun cancels all jobs associated with a run_id.
// Since jobs are tracked by job_id, this iterates to find matching run_ids.
// Comparisons use typed types.RunID values directly for type safety.
func (r *runController) StopRun(_ context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found bool
	for jobKey, job := range r.jobs {
		// Compare typed RunID values directly — no string conversion needed.
		if job.runID == req.RunID {
			job.cancel()
			delete(r.jobs, jobKey)
			found = true
		}
	}

	if !found {
		return fmt.Errorf("run %s not found", req.RunID)
	}

	return nil
}
