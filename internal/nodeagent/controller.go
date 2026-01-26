package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// MaxConcurrency is the maximum allowed concurrency for job execution.
// This prevents excessive resource consumption from misconfigured values.
const MaxConcurrency = 64

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id (not run_id) to support multiple jobs per run.
//
// Concurrency is enforced via the jobSem semaphore: callers must acquire a slot
// via AcquireSlot before claiming/starting work. On StartRun success, the
// controller releases the slot when the job completes. This prevents the node
// from claiming more jobs than it can execute concurrently.
//
// HTTP uploaders (diffUploader, artifactUploader, statusUploader) are created
// once at controller initialization and reused across all jobs. This avoids
// the overhead of creating new HTTP clients per upload call and enables
// connection pooling. The underlying http.Client is safe for concurrent use
// by multiple goroutines (per Go's net/http documentation).
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

	// diffUploader is a shared HTTP uploader for diff uploads.
	// Created once at controller initialization and reused across all jobs.
	// Safe for concurrent use by multiple goroutines.
	diffUploader *DiffUploader

	// artifactUploader is a shared HTTP uploader for artifact bundle uploads.
	// Created once at controller initialization and reused across all jobs.
	// Safe for concurrent use by multiple goroutines.
	artifactUploader *ArtifactUploader

	// statusUploader is a shared HTTP uploader for terminal status uploads.
	// Created once at controller initialization and reused across all jobs.
	// Safe for concurrent use by multiple goroutines.
	statusUploader *StatusUploader

	// jobImageNameSaver persists the resolved container image name that will be
	// used to execute a mod/heal job (jobs.mod_image).
	jobImageNameSaver *JobImageNameSaver
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
	// Cap at MaxConcurrency to prevent excessive resource consumption.
	if capacity > MaxConcurrency {
		slog.Warn("concurrency exceeds maximum, capping",
			"configured", capacity,
			"max", MaxConcurrency,
		)
		capacity = MaxConcurrency
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

// ensureUploaders lazily initializes the shared uploaders if they haven't been
// set. This provides backward compatibility for tests that construct runController
// directly without going through agent.New(). In production, uploaders are
// pre-initialized at agent startup for optimal HTTP connection reuse.
//
// Thread-safe: uses the controller's mutex to prevent concurrent initialization.
// Returns an error if uploader creation fails.
func (r *runController) ensureUploaders() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check and initialize each uploader if nil.
	// Creating uploaders reads the bearer token file and sets up HTTP transport.
	var err error

	if r.diffUploader == nil {
		r.diffUploader, err = NewDiffUploader(r.cfg)
		if err != nil {
			return fmt.Errorf("create diff uploader: %w", err)
		}
	}

	if r.artifactUploader == nil {
		r.artifactUploader, err = NewArtifactUploader(r.cfg)
		if err != nil {
			return fmt.Errorf("create artifact uploader: %w", err)
		}
	}

	if r.statusUploader == nil {
		r.statusUploader, err = NewStatusUploader(r.cfg)
		if err != nil {
			return fmt.Errorf("create status uploader: %w", err)
		}
	}

	if r.jobImageNameSaver == nil {
		r.jobImageNameSaver, err = NewJobImageNameSaver(r.cfg)
		if err != nil {
			return fmt.Errorf("create job image name saver: %w", err)
		}
	}

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
