package api

import (
	"encoding/json"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// RunRepoJobRecovery projects universal recovery metadata from job payloads.
type RunRepoJobRecovery struct {
	LoopKind                  string          `json:"loop_kind"`
	ErrorKind                 string          `json:"error_kind"`
	StrategyID                string          `json:"strategy_id,omitempty"`
	Confidence                *float64        `json:"confidence,omitempty"`
	Reason                    string          `json:"reason,omitempty"`
	Expectations              json.RawMessage `json:"expectations,omitempty"`
	CandidateSchemaID         string          `json:"candidate_schema_id,omitempty"`
	CandidateArtifactPath     string          `json:"candidate_artifact_path,omitempty"`
	CandidateValidationStatus string          `json:"candidate_validation_status,omitempty"`
	CandidateValidationError  string          `json:"candidate_validation_error,omitempty"`
	CandidatePromoted         *bool           `json:"candidate_promoted,omitempty"`
}

// RunRepoJob represents a job within a repo execution.
type RunRepoJob struct {
	JobID         domaintypes.JobID   `json:"job_id"`
	Name          string              `json:"name"`
	JobType       string              `json:"job_type"`
	JobImage      string              `json:"job_image"`
	NextID        *domaintypes.JobID  `json:"next_id"`
	NodeID        *domaintypes.NodeID `json:"node_id"`
	Status        store.JobStatus     `json:"status"`
	ExitCode      *int32              `json:"exit_code,omitempty"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	FinishedAt    *time.Time          `json:"finished_at,omitempty"`
	DurationMs    int64               `json:"duration_ms"`
	DisplayName   string              `json:"display_name,omitempty"`
	ActionSummary string              `json:"action_summary,omitempty"`
	BugSummary    string              `json:"bug_summary,omitempty"`
	Recovery      *RunRepoJobRecovery `json:"recovery,omitempty"`
}

// ListRunRepoJobsResponse is the response for GET /v1/runs/{run_id}/repos/{repo_id}/jobs.
type ListRunRepoJobsResponse struct {
	RunID   domaintypes.RunID  `json:"run_id"`
	RepoID  domaintypes.RepoID `json:"repo_id"`
	Attempt int32              `json:"attempt"`
	Jobs    []RunRepoJob       `json:"jobs"`
}
