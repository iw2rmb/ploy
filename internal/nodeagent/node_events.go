package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// NodeEventUploader posts run-scoped node events to the control plane.
type NodeEventUploader struct {
	*baseUploader
}

func NewNodeEventUploader(cfg Config) (*NodeEventUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &NodeEventUploader{baseUploader: base}, nil
}

// UploadRunEvent posts a single structured event for a run.
func (u *NodeEventUploader) UploadRunEvent(
	ctx context.Context,
	runID types.RunID,
	jobID *types.JobID,
	level string,
	message string,
	meta map[string]any,
) error {
	if runID.IsZero() {
		return fmt.Errorf("run_id is required")
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("event message is required")
	}

	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "error"
	}

	timeValue := time.Now().UTC().Format(time.RFC3339Nano)
	payload := struct {
		RunID  types.RunID `json:"run_id"`
		Events []struct {
			JobID   *types.JobID   `json:"job_id,omitempty"`
			Time    *string        `json:"time,omitempty"`
			Level   string         `json:"level"`
			Message string         `json:"message"`
			Meta    map[string]any `json:"meta,omitempty"`
		} `json:"events"`
	}{
		RunID: runID,
		Events: []struct {
			JobID   *types.JobID   `json:"job_id,omitempty"`
			Time    *string        `json:"time,omitempty"`
			Level   string         `json:"level"`
			Message string         `json:"message"`
			Meta    map[string]any `json:"meta,omitempty"`
		}{
			{
				JobID:   jobID,
				Time:    &timeValue,
				Level:   level,
				Message: message,
				Meta:    meta,
			},
		},
	}

	apiPath := fmt.Sprintf("/v1/nodes/%s/events", u.cfg.NodeID.String())
	resp, err := u.postJSON(ctx, apiPath, payload, http.StatusCreated, "upload node event")
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
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

	if err := r.ensureUploaders(); err != nil {
		slog.Warn("failed to initialize node event uploader", "run_id", runID, "error", err)
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

func (c *ClaimManager) ensureNodeEventUploader() (*NodeEventUploader, error) {
	c.eventUploaderOnce.Do(func() {
		c.eventUploader, c.eventUploaderErr = NewNodeEventUploader(c.cfg)
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
