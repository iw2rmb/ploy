package nodeagent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id (not run_id) to support multiple jobs per run.
type runController struct {
	mu sync.Mutex

	cfg Config

	// jobs tracks active jobs keyed by job_id.
	jobs map[string]*jobContext

	// activeBranch tracks the winning healing branch per run.
	// When a re-gate job for a branch succeeds, that branch becomes the
	// active execution path for subsequent jobs (e.g., mod-0, post-gate).
	// This allows rehydration to include prior diffs from the healing path
	// so downstream jobs see the healed baseline instead of the original.
	activeBranch map[string]string // keyed by run_id string
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

	if r.jobs == nil {
		r.jobs = make(map[string]*jobContext)
	}
	if r.activeBranch == nil {
		r.activeBranch = make(map[string]string)
	}

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

	// Clear any cached active branch for the stopped run to avoid leaks.
	if r.activeBranch != nil {
		delete(r.activeBranch, req.RunID.String())
	}

	if !found {
		return fmt.Errorf("run %s not found", req.RunID)
	}

	return nil
}

// setActiveBranch records the winning healing branch for a run.
// Once set, the active branch is not overwritten; the first successful
// re-gate branch becomes the canonical execution path for downstream jobs.
func (r *runController) setActiveBranch(runID types.RunID, branch string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activeBranch == nil {
		r.activeBranch = make(map[string]string)
	}
	key := runID.String()
	if _, exists := r.activeBranch[key]; exists {
		// Preserve the first winning branch to avoid oscillation if
		// multiple re-gates succeed due to scheduler races.
		return
	}
	r.activeBranch[key] = branch
}

// getActiveBranch returns the winning healing branch for a run, if any.
func (r *runController) getActiveBranch(runID types.RunID) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activeBranch == nil {
		return ""
	}
	return r.activeBranch[runID.String()]
}
