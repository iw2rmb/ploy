package types

import "time"

// RunSummary represents a v1 run summary with optional aggregated wave counts.
// It is the canonical domain shape for control-plane run summary responses and
// is shared between server handlers, CLI, and OpenAPI.
type RunSummary struct {
	ID               RunID      `json:"id"`
	Status           RunStatus  `json:"status"`
	MigID            MigID      `json:"mig_id"`
	MigName          string     `json:"mig_name,omitempty"`
	SpecID           SpecID     `json:"spec_id"`
	SpecName         string     `json:"spec_name,omitempty"`
	SpecSourceDomain string     `json:"spec_source_domain,omitempty"`
	SpecSourceRepo   string     `json:"spec_source_repo,omitempty"`
	RepoID           RepoID     `json:"repo_id,omitempty"`
	RepoURL          string     `json:"repo_url,omitempty"`
	BaseRef          string     `json:"base_ref,omitempty"`
	SourceCommitSHA  string     `json:"source_commit_sha,omitempty"`
	Attempt          int32      `json:"attempt,omitempty"`
	LastError        *string    `json:"last_error,omitempty"`
	CreatedBy        *string    `json:"created_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	Counts           *RunCounts `json:"run_counts,omitempty"`
}

// RunCounts aggregates the count of runs by status within a wave.
// DerivedStatus provides a single wave-level status derived from run states.
type RunCounts struct {
	Total         int32  `json:"total"`
	Queued        int32  `json:"queued"`
	Running       int32  `json:"running"`
	Success       int32  `json:"success"`
	Fail          int32  `json:"fail"`
	Cancelled     int32  `json:"cancelled"`
	DerivedStatus string `json:"derived_status"`
}
