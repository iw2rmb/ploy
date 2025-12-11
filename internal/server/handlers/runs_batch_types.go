package handlers

import (
	"context"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// NOTE: Run IDs in this file are KSUID-backed strings; run_repo IDs are NanoID(8)-backed strings.
// Both are now string types in the store layer; no UUID parsing is needed.

// RunBatchSummary represents a run with aggregated repo status counts.
// Used for list and detail responses.
// Uses domain types (RunID, RepoURL, GitRef) for type-safe serialization
// and validation at API boundaries.
type RunBatchSummary struct {
	ID         domaintypes.RunID   `json:"id"`
	Name       *string             `json:"name,omitempty"`
	Status     store.RunStatus     `json:"status"`
	RepoURL    domaintypes.RepoURL `json:"repo_url"`
	BaseRef    domaintypes.GitRef  `json:"base_ref"`
	TargetRef  domaintypes.GitRef  `json:"target_ref"`
	CreatedBy  *string             `json:"created_by,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
	Counts     *RunRepoCounts      `json:"repo_counts,omitempty"`
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
	DerivedStatus string `json:"derived_status"` // running, completed, failed, cancelled, pending
}

// Derived batch status constants exposed for API consumers.
// These represent the batch-level state computed from repo statuses.
const (
	// DerivedStatusPending indicates no repos have started (all pending or no repos).
	DerivedStatusPending = "pending"
	// DerivedStatusRunning indicates at least one repo is currently running.
	DerivedStatusRunning = "running"
	// DerivedStatusCompleted indicates all repos finished with succeeded or skipped.
	DerivedStatusCompleted = "completed"
	// DerivedStatusFailed indicates at least one repo failed (and none running).
	DerivedStatusFailed = "failed"
	// DerivedStatusCancelled indicates the batch was stopped and repos were cancelled.
	DerivedStatusCancelled = "cancelled"
)

// runToSummary converts a store.Run to a RunBatchSummary.
// Wraps raw store strings in domain types for type-safe API output.
// run.ID is now a string (KSUID), so no UUID conversion is needed.
func runToSummary(run store.Run) RunBatchSummary {
	summary := RunBatchSummary{
		// run.ID is now a string (KSUID); cast directly to domain type.
		ID:     domaintypes.RunID(run.ID),
		Name:   run.Name,
		Status: run.Status,
		// Wrap VCS fields in domain types; values are validated at input time.
		RepoURL:   domaintypes.RepoURL(run.RepoUrl),
		BaseRef:   domaintypes.GitRef(run.BaseRef),
		TargetRef: domaintypes.GitRef(run.TargetRef),
		CreatedBy: run.CreatedBy,
		CreatedAt: run.CreatedAt.Time,
	}

	if run.StartedAt.Valid {
		summary.StartedAt = &run.StartedAt.Time
	}
	if run.FinishedAt.Valid {
		summary.FinishedAt = &run.FinishedAt.Time
	}

	return summary
}

// getRunRepoCounts fetches and aggregates repo counts by status for a run.
// runID is now a KSUID-backed domain type.
func getRunRepoCounts(ctx context.Context, st store.Store, runID domaintypes.RunID) (*RunRepoCounts, error) {
	rows, err := st.CountRunReposByStatus(ctx, runID.String())
	if err != nil {
		return nil, err
	}

	counts := &RunRepoCounts{}
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case store.RunRepoStatusPending:
			counts.Pending = row.Count
		case store.RunRepoStatusRunning:
			counts.Running = row.Count
		case store.RunRepoStatusSucceeded:
			counts.Succeeded = row.Count
		case store.RunRepoStatusFailed:
			counts.Failed = row.Count
		case store.RunRepoStatusSkipped:
			counts.Skipped = row.Count
		case store.RunRepoStatusCancelled:
			counts.Cancelled = row.Count
		}
	}

	// Derive batch-level status from repo counts.
	counts.DerivedStatus = deriveBatchStatus(counts)

	return counts, nil
}

// deriveBatchStatus computes a single batch-level status from repo counts.
// The precedence order is:
//  1. cancelled — if any repo is cancelled (batch was explicitly stopped).
//  2. running — if any repo is currently running.
//  3. failed — if none running, and at least one repo failed.
//  4. completed — if all repos are in terminal states (succeeded/skipped) with no failures.
//  5. pending — if no repos have started yet (all pending, or no repos).
func deriveBatchStatus(counts *RunRepoCounts) string {
	// No repos in batch — treat as pending (batch has no work yet).
	if counts.Total == 0 {
		return DerivedStatusPending
	}

	// If any repo was cancelled, the batch was explicitly stopped.
	// This takes precedence because it represents user intent to abort.
	if counts.Cancelled > 0 {
		return DerivedStatusCancelled
	}

	// If any repo is currently running, the batch is actively running.
	if counts.Running > 0 {
		return DerivedStatusRunning
	}

	// At this point, no repos are running or cancelled.
	// Check if any repos failed — if so, the batch failed.
	if counts.Failed > 0 {
		return DerivedStatusFailed
	}

	// Calculate terminal repos (succeeded + skipped + failed + cancelled).
	// We already checked Failed and Cancelled above, so only succeeded/skipped remain.
	terminalCount := counts.Succeeded + counts.Skipped + counts.Failed + counts.Cancelled

	// If all repos are in terminal state and none failed, batch completed successfully.
	if terminalCount == counts.Total {
		return DerivedStatusCompleted
	}

	// Some repos are still pending (not started), batch is pending/waiting.
	return DerivedStatusPending
}

// isTerminalRunStatus returns true if the run status is terminal (no further transitions).
func isTerminalRunStatus(status store.RunStatus) bool {
	switch status {
	case store.RunStatusSucceeded, store.RunStatusFailed, store.RunStatusCanceled:
		return true
	default:
		return false
	}
}

// isTerminalRunRepoStatus returns true if the run repo status is terminal.
func isTerminalRunRepoStatus(status store.RunRepoStatus) bool {
	switch status {
	case store.RunRepoStatusSucceeded, store.RunRepoStatusFailed, store.RunRepoStatusSkipped, store.RunRepoStatusCancelled:
		return true
	default:
		return false
	}
}

// RunRepoResponse represents a single repo within a batch for API responses.
// Exposes repo URL, refs, attempt count, status, error, and timing fields.
// Uses domain types (RunRepoID, RunID, RepoURL, GitRef) for type-safe
// serialization at API boundaries.
type RunRepoResponse struct {
	ID         domaintypes.RunRepoID `json:"id"`
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoURL    domaintypes.RepoURL   `json:"repo_url"`
	BaseRef    domaintypes.GitRef    `json:"base_ref"`
	TargetRef  domaintypes.GitRef    `json:"target_ref"`
	Status     store.RunRepoStatus   `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

// runRepoToResponse converts a store.RunRepo to a RunRepoResponse.
// Wraps raw store strings in domain types for type-safe API output.
// rr.ID is now a string (NanoID); rr.RunID is a string (KSUID).
func runRepoToResponse(rr store.RunRepo) RunRepoResponse {
	resp := RunRepoResponse{
		// rr.ID is now a string (NanoID-backed); cast directly to domain type.
		ID: domaintypes.RunRepoID(rr.ID),
		// rr.RunID is a string (KSUID-backed); cast directly.
		RunID: domaintypes.RunID(rr.RunID),
		// Wrap VCS fields in domain types; values are validated at input time.
		RepoURL:   domaintypes.RepoURL(rr.RepoUrl),
		BaseRef:   domaintypes.GitRef(rr.BaseRef),
		TargetRef: domaintypes.GitRef(rr.TargetRef),
		// Use typed status instead of raw string for type safety.
		Status:    rr.Status,
		Attempt:   rr.Attempt,
		LastError: rr.LastError,
		CreatedAt: rr.CreatedAt.Time,
	}
	if rr.StartedAt.Valid {
		resp.StartedAt = &rr.StartedAt.Time
	}
	if rr.FinishedAt.Valid {
		resp.FinishedAt = &rr.FinishedAt.Time
	}
	return resp
}
