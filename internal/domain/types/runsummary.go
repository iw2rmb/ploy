package types

import "time"

// RunSummary represents a batch run summary with optional aggregated repo
// status counts. It is the canonical domain shape for control-plane run
// summary responses and is shared between server handlers, CLI, and OpenAPI.
type RunSummary struct {
	ID         RunID          `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Status     string         `json:"status"`
	RepoURL    string         `json:"repo_url"`
	BaseRef    string         `json:"base_ref"`
	TargetRef  string         `json:"target_ref"`
	CreatedBy  *string        `json:"created_by,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Counts     *RunRepoCounts `json:"repo_counts,omitempty"`
}

// RunRepoCounts aggregates the count of repos by status within a batch.
// DerivedStatus provides a single batch-level status derived from repo states.
type RunRepoCounts struct {
	Total         int32  `json:"total"`
	Pending       int32  `json:"pending"`
	Running       int32  `json:"running"`
	Succeeded     int32  `json:"succeeded"`
	Failed        int32  `json:"failed"`
	Skipped       int32  `json:"skipped"`
	Cancelled     int32  `json:"cancelled"`
	DerivedStatus string `json:"derived_status"`
}
