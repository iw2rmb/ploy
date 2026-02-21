package nodeagent

import (
	"context"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
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
	return s.postJSONWithRetry(ctx, fmt.Sprintf("/v1/jobs/%s/image", jobID.String()), map[string]any{"image": image}, "save job image name")
}
