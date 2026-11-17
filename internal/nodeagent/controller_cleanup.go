package nodeagent

import (
	"context"
	"log/slog"
	"time"
)

const (
	nodeCleanupInterval = time.Hour
	nodeCleanupTimeout  = 5 * time.Minute
)

func (r *runController) tryCleanup() {
	r.mu.Lock()
	if r.cleanupRunning || time.Since(r.lastCleanupSuccess) < nodeCleanupInterval {
		r.mu.Unlock()
		return
	}
	r.cleanupRunning = true
	r.mu.Unlock()

	success := false
	defer func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cleanupRunning = false
		if success {
			r.lastCleanupSuccess = time.Now()
		}
	}()

	// Agent.Run supplies the node lifetime; completed job cancellation must not
	// suppress the only cleanup opportunity after finalization.
	ctx, cancel := context.WithTimeout(r.nodeCtx, nodeCleanupTimeout)
	defer cancel()
	lock, err := tryAcquireMaintenanceExclusive(ctx)
	if err != nil {
		slog.Error("node cleanup maintenance lock failed", "error", err)
		return
	}
	if lock == nil {
		return
	}
	defer lock.Close()
	if err := r.cleanupPass(ctx); err != nil {
		slog.Error("node cleanup failed", "error", err)
		return
	}
	if err := ctx.Err(); err != nil {
		slog.Error("node cleanup cancelled", "error", err)
		return
	}
	success = true
}
