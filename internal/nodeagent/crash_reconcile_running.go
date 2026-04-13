package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

const recoveredStatusUploadTimeout = 10 * time.Second

// startRecoveredRunningMonitors reserves concurrency slots and starts one monitor
// goroutine per recovered running container.
func (c *ClaimManager) startRecoveredRunningMonitors(ctx context.Context, recovered []recoveredRunningContainer) {
	if c == nil || len(recovered) == 0 {
		return
	}
	for _, item := range recovered {
		if err := c.controller.AcquireSlot(ctx); err != nil {
			slog.Warn("skip recovered running container monitor: acquire slot failed",
				"run_id", item.RunID,
				"job_id", item.JobID,
				"container_id", item.ContainerID,
				"error", err,
			)
			continue
		}

		recoveredItem := item
		go c.monitorRecoveredRunningContainer(ctx, recoveredItem)
	}
}

func (c *ClaimManager) monitorRecoveredRunningContainer(ctx context.Context, recovered recoveredRunningContainer) {
	defer c.controller.ReleaseSlot()

	if err := c.waitAndUploadRecoveredContainer(ctx, recovered); err != nil {
		slog.Warn("recovered running container monitor failed",
			"run_id", recovered.RunID,
			"job_id", recovered.JobID,
			"container_id", recovered.ContainerID,
			"error", err,
		)
		c.emitRunException(
			recovered.RunID,
			jobIDPtr(recovered.JobID),
			"startup recovered-running monitor failed",
			err,
			map[string]any{
				"component":    "startup_reconcile",
				"container_id": recovered.ContainerID,
			},
		)
	}
}

func (c *ClaimManager) waitAndUploadRecoveredContainer(ctx context.Context, recovered recoveredRunningContainer) error {
	if c == nil || c.startupReconciler == nil {
		return errors.New("startup crash reconciler not configured")
	}

	terminal, err := c.startupReconciler.WaitRecoveredContainer(ctx, recovered.ContainerID)
	if err != nil {
		stats := types.NewRunStatsBuilder().
			ExitCode(-1).
			Error(err.Error()).
			MetadataEntry("source", "startup_reconcile").
			MetadataEntry("container_id", recovered.ContainerID).
			MustBuild()
		failureExitCode := int32(-1)
		if uploadErr := c.uploadRecoveredJobStatus(recovered.JobID, types.JobStatusError, &failureExitCode, stats); uploadErr != nil {
			return fmt.Errorf("wait container and upload failure status: %w (upload error: %v)", err, uploadErr)
		}
		return fmt.Errorf("wait recovered container: %w", err)
	}

	if stdoutLogs, stderrLogs, logsErr := c.startupReconciler.ReadContainerLogs(ctx, recovered.ContainerID); logsErr != nil {
		slog.Warn("failed to read recovered container logs",
			"run_id", recovered.RunID,
			"job_id", recovered.JobID,
			"container_id", recovered.ContainerID,
			"error", logsErr,
		)
	} else if len(stdoutLogs) > 0 || len(stderrLogs) > 0 {
		if err := c.uploadRecoveredLogs(recovered.RunID, recovered.JobID, stdoutLogs, stderrLogs); err != nil {
			slog.Warn("failed to upload recovered container logs",
				"run_id", recovered.RunID,
				"job_id", recovered.JobID,
				"container_id", recovered.ContainerID,
				"error", err,
			)
		}
	}

	exitCode := int32(terminal.ExitCode)
	status := lifecycle.JobStatusFromExitCode(int(exitCode))

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
		return fmt.Errorf("upload recovered container terminal status: %w", err)
	}
	return nil
}

func (c *ClaimManager) uploadRecoveredLogs(runID types.RunID, jobID types.JobID, stdoutLogs, stderrLogs []byte) error {
	logStreamer, err := NewLogStreamer(c.cfg, runID, jobID, nil)
	if err != nil {
		return fmt.Errorf("create recovered log streamer: %w", err)
	}
	if len(stdoutLogs) > 0 {
		if _, err := logStreamer.StdoutWriter().Write(stdoutLogs); err != nil {
			_ = logStreamer.Close()
			return fmt.Errorf("write recovered stdout logs: %w", err)
		}
	}
	if len(stderrLogs) > 0 {
		if _, err := logStreamer.StderrWriter().Write(stderrLogs); err != nil {
			_ = logStreamer.Close()
			return fmt.Errorf("write recovered stderr logs: %w", err)
		}
	}
	if err := logStreamer.Close(); err != nil {
		return fmt.Errorf("close recovered log streamer: %w", err)
	}
	return nil
}

func (c *ClaimManager) uploadRecoveredJobStatus(jobID types.JobID, status types.JobStatus, exitCode *int32, stats types.RunStats) error {
	uploader, err := c.ensureStatusUploader()
	if err != nil {
		return err
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), recoveredStatusUploadTimeout)
	defer cancel()
	if err := uploader.UploadJobStatusReconcile(statusCtx, jobID, status.String(), exitCode, stats); err != nil {
		return fmt.Errorf("upload job status: %w", err)
	}
	return nil
}

func (c *ClaimManager) ensureStatusUploader() (*baseUploader, error) {
	c.statusUploaderOnce.Do(func() {
		c.statusUploader, c.statusUploaderErr = newBaseUploader(c.cfg)
	})
	if c.statusUploaderErr != nil {
		return nil, c.statusUploaderErr
	}
	return c.statusUploader, nil
}
