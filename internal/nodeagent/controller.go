package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const MaxConcurrency = 64

// runController implements the RunController interface for managing runs.
// Runs are tracked by job_id to support multiple jobs per run.
// Concurrency is enforced via the jobSem semaphore.
type runController struct {
	mu      sync.Mutex
	sweepMu sync.Mutex

	cfg Config

	jobs   map[types.JobID]*jobContext
	jobSem chan struct{}

	uploader *baseUploader

	nodeCtx            context.Context
	cleanupPass        func(context.Context) error
	cleanupRunning     bool
	lastCleanupSuccess time.Time
	imageLastUsed      map[string]time.Time
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

// RunSlot owns a concurrency permit and an independent shared maintenance lock.
// The caller releases it unless StartRun succeeds and transfers ownership.
type RunSlot struct {
	owner *runController
	lock  *os.File
}

func (r *runController) AcquireSlot(ctx context.Context) (*RunSlot, error) {
	r.mu.Lock()
	r.initJobSem()
	sem := r.jobSem
	r.mu.Unlock()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	lock, err := acquireMaintenanceActivity(ctx)
	if err != nil {
		<-sem
		return nil, err
	}
	return &RunSlot{owner: r, lock: lock}, nil
}

func (r *runController) ReleaseSlot(slot *RunSlot) {
	if slot == nil || slot.owner != r {
		panic("invalid run slot owner")
	}
	if err := slot.releaseActivity(); err != nil {
		slog.Error("release maintenance activity", "error", err)
	}
	<-r.jobSem
}

func (slot *RunSlot) releaseActivity() error {
	if slot.lock == nil {
		return nil
	}
	err := slot.lock.Close()
	slot.lock = nil
	return err
}

// FinishRunSlot keeps the permit until the after-job hook finishes. Empty
// claims use ReleaseSlot instead, so polling cannot trigger cleanup.
func (r *runController) FinishRunSlot(slot *RunSlot) {
	defer r.ReleaseSlot(slot)
	if err := slot.releaseActivity(); err != nil {
		slog.Error("release maintenance activity before cleanup", "error", err)
		return
	}
	r.tryCleanup()
}

func (r *runController) beforeJob(ctx context.Context, slot *RunSlot) error {
	if err := slot.releaseActivity(); err != nil {
		return fmt.Errorf("release maintenance activity before cleanup: %w", err)
	}
	r.tryCleanup()
	lock, err := acquireMaintenanceActivity(ctx)
	if err != nil {
		return err
	}
	slot.lock = lock
	return nil
}

// StartRun accepts a run start request and initiates execution.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest, slot *RunSlot) error {
	if slot == nil || slot.owner != r {
		return fmt.Errorf("run slot belongs to another controller")
	}
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
		defer r.FinishRunSlot(slot)
		if err := r.beforeJob(runCtx, slot); err != nil {
			r.uploadFailureStatus(runCtx, req, err, 0)
			r.mu.Lock()
			delete(r.jobs, req.JobID)
			r.mu.Unlock()
			return
		}
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
