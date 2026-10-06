package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

func (c *ClaimManager) runStartupReconcile(ctx context.Context) error {
	if c == nil {
		return errors.New("claim manager not configured")
	}

	c.startupOnce.Do(func() {
		c.startupErr = c.runStartupReconcilePass(ctx)
	})

	return c.startupErr
}

func (c *ClaimManager) runStartupReconcilePass(ctx context.Context) error {
	if c.startupReconciler == nil {
		return errors.New("startup crash reconciler not configured")
	}

	snapshot, err := c.startupReconciler.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discover startup crash containers: %w", err)
	}

	slog.Info(
		"startup crash reconcile snapshot",
		"running_count", len(snapshot.Running),
		"recent_terminal_count", len(snapshot.RecentTerminal),
	)

	c.startRecoveredRunningMonitors(ctx, snapshot.Running)
	c.reconcileRecoveredTerminalContainers(ctx, snapshot.RecentTerminal)
	c.sweepAbandonedRuntimeIfIdle()
	return nil
}

func (c *ClaimManager) sweepAbandonedRuntimeIfIdle() {
	if sweeper, ok := c.controller.(interface{ sweepAbandonedRuntimeIfIdle() }); ok {
		sweeper.sweepAbandonedRuntimeIfIdle()
	}
}

func (c *ClaimManager) reconcileRecoveredTerminalContainers(ctx context.Context, recovered []recoveredContainer) {
	if c == nil || len(recovered) == 0 {
		return
	}

	for _, item := range recovered {
		if err := c.reconcileRecoveredTerminalContainer(ctx, item); err != nil {
			slog.Warn(
				"startup terminal container reconciliation failed",
				"run_id", item.RunID,
				"job_id", item.JobID,
				"container_id", item.ContainerID,
				"error", err,
			)
			c.emitRunException(
				item.RunID,
				jobIDPtr(item.JobID),
				"startup recovered-terminal reconciliation failed",
				err,
				map[string]any{
					"component":    "startup_reconcile",
					"container_id": item.ContainerID,
				},
			)
		}
	}
}

func (c *ClaimManager) reconcileRecoveredTerminalContainer(ctx context.Context, recovered recoveredContainer) error {
	if c == nil || c.startupReconciler == nil {
		return errors.New("startup crash reconciler not configured")
	}

	terminal, err := c.startupReconciler.WaitRecoveredContainer(ctx, recovered.ContainerID)
	if err != nil {
		return fmt.Errorf("wait recovered terminal container: %w", err)
	}

	if err := c.uploadRecoveredTerminalStatus(recovered.JobID, recovered.ContainerID, terminal, recovered.ResumeCount); err != nil {
		return fmt.Errorf("upload recovered terminal status: %w", err)
	}
	return nil
}
