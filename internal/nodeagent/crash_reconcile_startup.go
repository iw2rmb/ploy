package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	types "github.com/iw2rmb/ploy/internal/domain/types"
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
	return nil
}

func (c *ClaimManager) reconcileRecoveredTerminalContainers(ctx context.Context, recovered []recoveredTerminalContainer) {
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

func (c *ClaimManager) reconcileRecoveredTerminalContainer(ctx context.Context, recovered recoveredTerminalContainer) error {
	if c == nil || c.startupReconciler == nil {
		return errors.New("startup crash reconciler not configured")
	}

	terminal, err := c.startupReconciler.WaitRecoveredContainer(ctx, recovered.ContainerID)
	if err != nil {
		return fmt.Errorf("wait recovered terminal container: %w", err)
	}

	exitCode := int32(terminal.ExitCode)
	status := JobStatusFail
	if exitCode == 0 {
		status = JobStatusSuccess
	}

	durationMs := int64(0)
	if !terminal.StartedAt.IsZero() && !terminal.FinishedAt.IsZero() && terminal.FinishedAt.After(terminal.StartedAt) {
		durationMs = terminal.FinishedAt.Sub(terminal.StartedAt).Milliseconds()
	}
	stats := types.NewRunStatsBuilder().
		ExitCode(int(exitCode)).
		DurationMs(durationMs).
		MetadataEntry("source", "startup_reconcile").
		MetadataEntry("container_id", recovered.ContainerID).
		MustBuild()

	if err := c.uploadRecoveredJobStatus(recovered.JobID, status, &exitCode, stats); err != nil {
		return fmt.Errorf("upload recovered terminal status: %w", err)
	}
	return nil
}
