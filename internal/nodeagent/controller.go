package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/nodeagent/git"
)

const MaxConcurrency = 64

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id to support multiple jobs per run.
// Concurrency is enforced via the jobSem semaphore.
type runController struct {
	mu sync.Mutex

	cfg Config

	jobs   map[types.JobID]*jobContext
	jobSem chan struct{}

	// uploader is the shared HTTP uploader used for all upload operations.
	// Individual fields exist for clarity at call sites but point to the same instance.
	diffUploader      *baseUploader
	artifactUploader  *baseUploader
	statusUploader    *baseUploader
	jobImageNameSaver *baseUploader
	nodeEventUploader *baseUploader

	// httpClient is the shared HTTP client for components created during job execution
	// (e.g., log streamer). Created once at init to avoid duplicate TLS/token I/O.
	httpClient *http.Client

	// Factory functions for MR creation dependencies.
	// Defaults are set in New(); tests can override.
	newPusher   func() git.Pusher
	newMRClient func() mrCreator
}

type jobContext struct {
	runID  types.RunID
	jobID  types.JobID
	cancel context.CancelFunc
}

func (r *runController) initJobSem() {
	if r.jobSem != nil {
		return
	}
	capacity := r.cfg.Concurrency
	if capacity < 1 {
		capacity = 1
	}
	if capacity > MaxConcurrency {
		slog.Warn("concurrency exceeds maximum, capping", "configured", capacity, "max", MaxConcurrency)
		capacity = MaxConcurrency
	}
	r.jobSem = make(chan struct{}, capacity)
}

// AcquireSlot blocks until a concurrency slot is available or the context is canceled.
func (r *runController) AcquireSlot(ctx context.Context) error {
	r.mu.Lock()
	r.initJobSem()
	sem := r.jobSem
	r.mu.Unlock()

	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot frees a previously acquired concurrency slot.
func (r *runController) ReleaseSlot() {
	r.mu.Lock()
	sem := r.jobSem
	r.mu.Unlock()

	if sem == nil {
		return
	}
	<-sem
}

// StartRun accepts a run start request and initiates execution.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[req.JobID]; exists {
		return fmt.Errorf("job %s already exists", req.JobID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.jobs[req.JobID] = &jobContext{
		runID:  req.RunID,
		jobID:  req.JobID,
		cancel: cancel,
	}

	r.startRemoteCancellationWatch(runCtx, req, cancel)
	go func() {
		defer cancel()
		r.executeRun(runCtx, req)
	}()

	return nil
}

// StopRun cancels all jobs associated with a run_id.
func (r *runController) StopRun(_ context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found bool
	for jobKey, job := range r.jobs {
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
