package api

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// RunJob represents a job within a run.
type RunJob struct {
	JobID       domaintypes.JobID     `json:"job_id"`
	Name        string                `json:"name"`
	JobType     domaintypes.JobType   `json:"job_type"`
	JobImage    string                `json:"job_image"`
	RepoShaIn   string                `json:"repo_sha_in,omitempty"`
	RepoShaOut  string                `json:"repo_sha_out,omitempty"`
	NextID      *domaintypes.JobID    `json:"next_id"`
	NodeID      *domaintypes.NodeID   `json:"node_id"`
	Status      domaintypes.JobStatus `json:"status"`
	ExitCode    *int32                `json:"exit_code,omitempty"`
	StartedAt   *time.Time            `json:"started_at,omitempty"`
	FinishedAt  *time.Time            `json:"finished_at,omitempty"`
	DurationMs  int64                 `json:"duration_ms"`
	DisplayName string                `json:"display_name,omitempty"`
	BugSummary  string                `json:"bug_summary,omitempty"`
	// Build-gate stack detection fields (gate jobs only).
	// Populated from gate metadata's detected_stack when present.
	Lang    string `json:"lang,omitempty"`
	Version string `json:"version,omitempty"`
	Tooling string `json:"tooling,omitempty"`
}

// ListRunJobsResponse is the response for GET /v1/runs/{run_id}/jobs.
type ListRunJobsResponse struct {
	RunID   domaintypes.RunID  `json:"run_id"`
	RepoID  domaintypes.RepoID `json:"repo_id"`
	Attempt int32              `json:"attempt"`
	Jobs    []RunJob           `json:"jobs"`
}
