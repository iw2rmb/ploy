package nodeagent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// nodeEventTimeNow returns the current time formatted for node event payloads.
func nodeEventTimeNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func eventLevelFromErr(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "warn"
	}
	return "error"
}

func cloneEventMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func (r *runController) emitRunException(req StartRunRequest, message string, err error, meta map[string]any) {
	eventMeta := cloneEventMeta(meta)
	if err != nil {
		eventMeta["error"] = err.Error()
	}
	r.emitRunEvent(req.RunID, jobIDPtr(req.JobID), eventLevelFromErr(err), message, eventMeta)
}

func (r *runController) emitRunEvent(runID types.RunID, jobID *types.JobID, level string, message string, meta map[string]any) {
	if runID.IsZero() {
		return
	}

	if r.nodeEventUploader == nil {
		slog.Warn("node event uploader is not initialized", "run_id", runID)
		return
	}

	eventCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.nodeEventUploader.UploadRunEvent(eventCtx, runID, jobID, level, message, meta); err != nil {
		slog.Warn("failed to upload node event", "run_id", runID, "job_id", derefJobID(jobID), "error", err)
	}
}

func (c *ClaimManager) emitRunException(runID types.RunID, jobID *types.JobID, message string, err error, meta map[string]any) {
	if runID.IsZero() {
		return
	}

	eventMeta := cloneEventMeta(meta)
	if err != nil {
		eventMeta["error"] = err.Error()
	}

	uploader, uploaderErr := c.ensureNodeEventUploader()
	if uploaderErr != nil {
		slog.Warn("failed to initialize claim-loop node event uploader", "run_id", runID, "error", uploaderErr)
		return
	}

	eventCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if uploadErr := uploader.UploadRunEvent(eventCtx, runID, jobID, eventLevelFromErr(err), message, eventMeta); uploadErr != nil {
		slog.Warn("failed to upload claim-loop node event", "run_id", runID, "job_id", derefJobID(jobID), "error", uploadErr)
	}
}

func (c *ClaimManager) ensureNodeEventUploader() (*baseUploader, error) {
	c.eventUploaderOnce.Do(func() {
		c.eventUploader, c.eventUploaderErr = newBaseUploader(c.cfg)
	})
	if c.eventUploaderErr != nil {
		return nil, c.eventUploaderErr
	}
	return c.eventUploader, nil
}

func jobIDPtr(jobID types.JobID) *types.JobID {
	if jobID.IsZero() {
		return nil
	}
	v := jobID
	return &v
}

func derefJobID(jobID *types.JobID) string {
	if jobID == nil {
		return ""
	}
	return jobID.String()
}
