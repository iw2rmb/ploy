package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cenkalti/backoff/v5"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	wfbackoff "github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// --- Job status types ---

// JobStatus represents the terminal status of a job execution.
type JobStatus string

const (
	JobStatusSuccess   JobStatus = "Success"
	JobStatusFail      JobStatus = "Fail"
	JobStatusCancelled JobStatus = "Cancelled"
)

func (s JobStatus) String() string { return string(s) }

// DiffModType represents the mod_type value used to tag diffs.
type DiffModType string

const (
	DiffModTypeMod     DiffModType = "mod"
	DiffModTypeHealing DiffModType = "healing"
)

func (t DiffModType) String() string { return string(t) }

// --- Job image name persistence ---

// SaveJobImageName persists the resolved container image name for a job to the control plane.
func (r *runController) SaveJobImageName(ctx context.Context, jobID types.JobID, image string) error {
	if err := r.ensureUploaders(); err != nil {
		return err
	}
	if r.jobImageNameSaver == nil {
		return fmt.Errorf("job image name saver not initialized")
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("image is empty")
	}
	return r.jobImageNameSaver.SaveJobImageName(ctx, jobID, image)
}

// JobImageNameSaver persists the container image name that will be used to execute a job.
type JobImageNameSaver struct {
	*baseUploader
}

func NewJobImageNameSaver(cfg Config) (*JobImageNameSaver, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &JobImageNameSaver{baseUploader: base}, nil
}

// SaveJobImageName persists the resolved container image name for a job.
func (s *JobImageNameSaver) SaveJobImageName(ctx context.Context, jobID types.JobID, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("image is empty")
	}

	body, err := json.Marshal(map[string]any{"image": image})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	apiPath := fmt.Sprintf("/v1/jobs/%s/image", jobID.String())
	url := MustBuildURL(s.cfg.ServerURL, apiPath)

	policy := wfbackoff.StatusUploaderPolicy()
	logger := slog.Default()

	attempt := 0
	uploadOp := func() error {
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			logger.Warn("save job image name request failed, retrying", "job_id", jobID, "attempt", attempt, "error", err)
			return fmt.Errorf("send request: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			return nil
		}

		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			logger.Warn("save job image name received 5xx, retrying", "job_id", jobID, "attempt", attempt, "status_code", resp.StatusCode)
			return fmt.Errorf("save failed: status %d: %s", resp.StatusCode, string(respBody))
		}

		return backoff.Permanent(fmt.Errorf("save failed: status %d: %s", resp.StatusCode, string(respBody)))
	}

	return wfbackoff.RunWithBackoff(ctx, policy, logger, uploadOp)
}
