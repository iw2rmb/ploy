package nodeagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id (not run_id) to support multiple jobs per run.
type runController struct {
	mu sync.Mutex

	cfg Config

	// jobs tracks active jobs by job_id.
	jobs map[string]*jobContext

	// activePaths holds the selected execution path per run after a successful
	// re-gate. Downstream jobs (mod-0, post-gate) without an explicit path in
	// their job name use this to rehydrate from the healed baseline instead of
	// the original failing baseline.
	activePaths map[string]string // run_id -> path_id
}

type jobContext struct {
	runID  string
	jobID  string
	cancel context.CancelFunc
}

// setActivePath records the selected execution path for a run after a
// successful re-gate job.
func (r *runController) setActivePath(runID types.RunID, pathID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activePaths == nil {
		r.activePaths = make(map[string]string)
	}
	r.activePaths[runID.String()] = pathID
}

// getActivePath returns the execution path selected for this run, if any.
func (r *runController) getActivePath(runID types.RunID) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activePaths == nil {
		return ""
	}
	return r.activePaths[runID.String()]
}

// StartRun accepts a run start request and initiates execution.
// Tracks by job_id to allow multiple jobs within the same run_id (e.g. pre-gate, mod-0).
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobKey := req.JobID.String()
	if _, exists := r.jobs[jobKey]; exists {
		return fmt.Errorf("job %s already exists", req.JobID)
	}

	// Create a cancellable context for this job, derived from caller.
	runCtx, cancel := context.WithCancel(ctx)
	r.jobs[jobKey] = &jobContext{
		runID:  req.RunID.String(),
		jobID:  jobKey,
		cancel: cancel,
	}

	// Execute the run/job in a goroutine.
	go r.executeRun(runCtx, req)

	return nil
}

// StopRun cancels all jobs associated with a run_id.
// Since jobs are tracked by job_id, this iterates to find matching run_ids.
func (r *runController) StopRun(_ context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found bool
	for jobKey, job := range r.jobs {
		if job.runID == req.RunID.String() {
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
