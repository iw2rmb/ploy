package runs

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Summary represents a run summary from the control-plane.
// It mirrors the server's RunBatchSummary type for CLI consumption.
// Uses domain type (RunID) for type-safe identification.
type Summary struct {
	ID         domaintypes.RunID `json:"id"` // Run ID (KSUID-backed)
	Name       *string           `json:"name,omitempty"`
	Status     string            `json:"status"`
	RepoURL    string            `json:"repo_url"`
	BaseRef    string            `json:"base_ref"`
	TargetRef  string            `json:"target_ref"`
	CreatedBy  *string           `json:"created_by,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	Counts     *RepoCounts       `json:"repo_counts,omitempty"`
}

// RepoCounts aggregates the count of repos by status within a run.
// DerivedStatus provides a single run-level status derived from repo states.
type RepoCounts struct {
	Total         int32  `json:"total"`
	Pending       int32  `json:"pending"`
	Running       int32  `json:"running"`
	Succeeded     int32  `json:"succeeded"`
	Failed        int32  `json:"failed"`
	Skipped       int32  `json:"skipped"`
	Cancelled     int32  `json:"cancelled"`
	DerivedStatus string `json:"derived_status"`
}
