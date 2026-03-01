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

// DiffJobType represents the job_type value used to tag diffs.
type DiffJobType string

const (
	DiffJobTypeMod DiffJobType = "mig"
)

func (t DiffJobType) String() string { return string(t) }

// --- Job image name persistence ---

// SaveJobImageName persists the resolved container image name for a job to the control plane.
func (r *runController) SaveJobImageName(ctx context.Context, jobID types.JobID, image string) error {
	if r.jobImageNameSaver == nil {
		return fmt.Errorf("job image name saver not initialized")
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("image is empty")
	}
	return r.jobImageNameSaver.SaveJobImageName(ctx, jobID, image)
}
