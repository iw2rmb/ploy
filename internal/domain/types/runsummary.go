package types

import "time"

// RunSummary represents a v1 run summary with optional aggregated repo
// status counts. It is the canonical domain shape for control-plane run
// summary responses and is shared between server handlers, CLI, and OpenAPI.
type RunSummary struct {
	ID         RunID          `json:"id"`
	Status     string         `json:"status"`
	MigID      MigID          `json:"mig_id"`
	SpecID     SpecID         `json:"spec_id"`
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
	Queued        int32  `json:"queued"`
	Running       int32  `json:"running"`
	Success       int32  `json:"success"`
	Fail          int32  `json:"fail"`
	Cancelled     int32  `json:"cancelled"`
	DerivedStatus string `json:"derived_status"`
}
