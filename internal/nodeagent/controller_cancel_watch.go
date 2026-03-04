package nodeagent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	remoteCancellationPollInterval = 3 * time.Second
	remoteCancellationPollTimeout  = 5 * time.Second
)

func (r *runController) startRemoteCancellationWatch(ctx context.Context, req StartRunRequest, cancel context.CancelFunc) {
	if r == nil || r.statusUploader == nil || req.JobID.IsZero() || cancel == nil {
		return
	}
	go r.watchRemoteCancellation(ctx, req, cancel)
}

func (r *runController) watchRemoteCancellation(ctx context.Context, req StartRunRequest, cancel context.CancelFunc) {
	ticker := time.NewTicker(remoteCancellationPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		statusCtx, timeoutCancel := context.WithTimeout(ctx, remoteCancellationPollTimeout)
		status, err := r.statusUploader.GetJobStatus(statusCtx, req.JobID)
		timeoutCancel()
		if err == nil && strings.EqualFold(strings.TrimSpace(status), types.JobStatusCancelled.String()) {
			slog.Info(
				"control plane cancelled running job; stopping local execution",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"job_type", req.JobType,
			)
			cancel()
			return
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Debug(
				"job cancellation poll failed",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", err,
			)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
