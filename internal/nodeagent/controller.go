package nodeagent

import (
	"context"
	"fmt"
	"sync"
)

// runController implements the RunController interface for managing runs.
type runController struct {
	mu   sync.Mutex
	cfg  Config
	runs map[string]*runContext
}

type runContext struct {
	runID  string
	cancel context.CancelFunc
}

// StartRun accepts a run start request and initiates execution.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runs[req.RunID]; exists {
		return fmt.Errorf("run %s already exists", req.RunID)
	}

	// Create a cancellable context for this run, derived from caller.
	runCtx, cancel := context.WithCancel(ctx)
	r.runs[req.RunID] = &runContext{
		runID:  req.RunID,
		cancel: cancel,
	}

	// In the skeleton, we just accept the run without executing it.
	// Actual execution will be implemented in subsequent tasks.
	go r.executeRun(runCtx, req)

	return nil
}

// StopRun cancels a running job.
func (r *runController) StopRun(_ context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[req.RunID]
	if !exists {
		return fmt.Errorf("run %s not found", req.RunID)
	}

	run.cancel()
	delete(r.runs, req.RunID)

	return nil
}
