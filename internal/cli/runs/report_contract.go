package runs

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// RunReport is the canonical status report contract shared by all status renderers.
type RunReport struct {
	RunID   domaintypes.RunID  `json:"run_id"`
	MigID   domaintypes.MigID  `json:"mig_id"`
	MigName string             `json:"mig_name"`
	SpecID  domaintypes.SpecID `json:"spec_id"`
	Repos   []RunEntry         `json:"repos"`
}

// RunEntry captures repo-level status, job graph data, and report links for a single repo attempt.
type RunEntry struct {
	RepoID      domaintypes.MigRepoID     `json:"repo_id"`
	RepoURL     string                    `json:"repo_url"`
	BaseRef     string                    `json:"base_ref"`
	TargetRef   string                    `json:"target_ref"`
	Attempt     int32                     `json:"attempt"`
	Status      domaintypes.RunRepoStatus `json:"status"`
	LastError   *string                   `json:"last_error,omitempty"`
	BuildLogURL string                    `json:"build_log_url,omitempty"`
	PatchURL    string                    `json:"patch_url,omitempty"`
	Jobs        []RunJobEntry             `json:"jobs"`
}

// RunJobEntry is one row in the follow-style job graph.
type RunJobEntry struct {
	JobID         domaintypes.JobID     `json:"job_id"`
	JobType       domaintypes.JobType   `json:"job_type"`
	JobImage      string                `json:"job_image"`
	NodeID        *domaintypes.NodeID   `json:"node_id,omitempty"`
	Status        domaintypes.JobStatus `json:"status"`
	ExitCode      *int32                `json:"exit_code,omitempty"`
	StartedAt     *time.Time            `json:"started_at,omitempty"`
	FinishedAt    *time.Time            `json:"finished_at,omitempty"`
	DurationMs    int64                 `json:"duration_ms"`
	DisplayName   string                `json:"display_name,omitempty"`
	ActionSummary string                `json:"action_summary,omitempty"`
	BugSummary    string                `json:"bug_summary,omitempty"`
	Recovery      *RunJobRecovery       `json:"recovery,omitempty"`
	Artifacts     []RunJobArtifact      `json:"artifacts,omitempty"`
	BuildLogURL   string                `json:"build_log_url,omitempty"`
	PatchURL      string                `json:"patch_url,omitempty"`
}

// RunJobRecovery projects recovery classifier fields surfaced by repo job APIs.
type RunJobRecovery = modsapi.RunRepoJobRecovery

// RunJobArtifact is the per-job artifact view emitted by run status JSON.
// It includes the CID plus a lookup URL that resolves artifact bundle metadata.
type RunJobArtifact struct {
	Name      string `json:"name"`
	CID       string `json:"cid"`
	LookupURL string `json:"lookup_url,omitempty"`
}
