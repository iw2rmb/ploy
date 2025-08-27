package jobs

import (
	"github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
)

// CreateJobRequest represents a request to create an asynchronous job
type CreateJobRequest struct {
	TarArchive   string                `json:"tar_archive"`
	RecipeConfig executor.RecipeConfig `json:"recipe_config"`
}

// JobResponse represents a job creation response
type JobResponse struct {
	JobID string `json:"job_id"`
}

// JobStatus represents job status information
type JobStatus struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`    // pending, running, completed, failed
	Progress  int    `json:"progress"`  // 0-100
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
	Error     string `json:"error,omitempty"`
}