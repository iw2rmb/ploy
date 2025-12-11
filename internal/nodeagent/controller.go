package nodeagent

import (
	"context"
	"fmt"
	"sync"
)

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id (not run_id) to support multiple jobs per run.
type runController struct {
	mu sync.Mutex

	cfg Config

	// jobs tracks active jobs by job_id.
	jobs map[string]*jobContext
}

type jobContext struct {
	runID  string
	jobID  string
	cancel context.CancelFunc
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
