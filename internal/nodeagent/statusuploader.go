package nodeagent

import (
	"context"
	"fmt"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// StatusUploader uploads terminal status and stats to the control-plane server.
type StatusUploader struct {
	*baseUploader
}

// NewStatusUploader creates a new status uploader.
func NewStatusUploader(cfg Config) (*StatusUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &StatusUploader{baseUploader: base}, nil
}

// UploadJobStatus uploads terminal status and stats to the job-level endpoint.
func (u *StatusUploader) UploadJobStatus(ctx context.Context, jobID types.JobID, status string, exitCode *int32, stats types.RunStats) error {
	payload := map[string]any{"status": status}
	if exitCode != nil {
		payload["exit_code"] = *exitCode
	}
	if stats != nil {
		payload["stats"] = stats
	}
	return u.postJSONWithRetry(ctx, fmt.Sprintf("/v1/jobs/%s/complete", jobID), payload, "upload job status")
}
