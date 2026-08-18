package api

import (
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

type JobStatusResponse struct {
	JobID       types.JobID     `json:"job_id"`
	RunID       types.RunID     `json:"run_id"`
	RepoID      types.RepoID    `json:"repo_id"`
	Attempt     int32           `json:"attempt"`
	Name        string          `json:"name"`
	JobType     types.JobType   `json:"job_type"`
	Status      types.JobStatus `json:"status"`
	JobImage    string          `json:"job_image"`
	NodeID      *types.NodeID   `json:"node_id"`
	ExitCode    *int32          `json:"exit_code"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
	DurationMs  int64           `json:"duration_ms"`
	RepoShaIn   string          `json:"repo_sha_in"`
	RepoShaOut  string          `json:"repo_sha_out"`
	RepoShaIn8  string          `json:"repo_sha_in8"`
	RepoShaOut8 string          `json:"repo_sha_out8"`
}

type JobListItem struct {
	JobID      types.JobID     `json:"job_id"`
	Name       string          `json:"name"`
	JobType    types.JobType   `json:"job_type"`
	Status     types.JobStatus `json:"status"`
	DurationMs int64           `json:"duration_ms"`
	JobImage   string          `json:"job_image"`
	NodeID     *types.NodeID   `json:"node_id"`
	MigName    string          `json:"mig_name"`
	RunID      types.RunID     `json:"run_id"`
	RepoID     types.RepoID    `json:"repo_id"`
}

type JobListResponse struct {
	Jobs  []JobListItem `json:"jobs"`
	Total int64         `json:"total"`
}
