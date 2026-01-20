package handlers

import (
	"context"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// NOTE: Run IDs in this file are KSUID-backed strings; run_repo IDs are NanoID(8)-backed strings.
// Both are now string types in the store layer; no UUID parsing is needed.

// RunSummary aliases the canonical domain RunSummary for server handlers.
type RunSummary = domaintypes.RunSummary

// RunRepoCounts aliases the canonical domain RunRepoCounts for server handlers.
type RunRepoCounts = domaintypes.RunRepoCounts

// Derived batch status constants exposed for API consumers.
// These represent the batch-level state computed from repo statuses.
const (
	// DerivedStatusPending indicates no repos have started (all queued or no repos).
	DerivedStatusPending = "pending"
	// DerivedStatusRunning indicates at least one repo is currently running.
	DerivedStatusRunning = "running"
	// DerivedStatusCompleted indicates all repos finished with no failures.
	DerivedStatusCompleted = "completed"
	// DerivedStatusFailed indicates at least one repo failed (and none running).
	DerivedStatusFailed = "failed"
	// DerivedStatusCancelled indicates the batch was stopped and repos were cancelled.
	DerivedStatusCancelled = "cancelled"
)

// runToSummary converts a store.Run to a RunSummary.
// Wraps raw store strings in domain types for type-safe API output.
// run.ID is now a string (KSUID), so no UUID conversion is needed.
func runToSummary(run store.Run) RunSummary {
	summary := RunSummary{
		// run.ID is now a string (KSUID); cast directly to domain type.
		ID:        run.ID,
		Status:    string(run.Status),
		ModID:     run.ModID,
		SpecID:    run.SpecID,
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
	rows, err := st.CountRunReposByStatus(ctx, runID)
	if err != nil {
		return nil, err
	}

	counts := &RunRepoCounts{}
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case store.RunRepoStatusQueued:
			counts.Queued = row.Count
		case store.RunRepoStatusRunning:
			counts.Running = row.Count
		case store.RunRepoStatusSuccess:
			counts.Success = row.Count
		case store.RunRepoStatusFail:
			counts.Fail = row.Count
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
//  4. completed — if all repos are in terminal states (success/cancelled) with no failures.
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
	if counts.Fail > 0 {
		return DerivedStatusFailed
	}

	terminalCount := counts.Success + counts.Fail + counts.Cancelled

	// If all repos are in terminal state and none failed, batch completed successfully.
	if terminalCount == counts.Total {
		return DerivedStatusCompleted
	}

	// Some repos are still queued (not started), batch is pending/waiting.
	return DerivedStatusPending
}

// isTerminalRunStatus returns true if the run status is terminal (no further transitions).
func isTerminalRunStatus(status store.RunStatus) bool {
	switch status {
	case store.RunStatusFinished, store.RunStatusCancelled:
		return true
	default:
		return false
	}
}

// isTerminalRunRepoStatus returns true if the run repo status is terminal.
func isTerminalRunRepoStatus(status store.RunRepoStatus) bool {
	switch status {
	case store.RunRepoStatusSuccess, store.RunRepoStatusFail, store.RunRepoStatusCancelled:
		return true
	default:
		return false
	}
}

// RunRepoResponse represents a single repo within a batch for API responses.
// Exposes repo URL, refs, attempt count, status, error, and timing fields.
// v1 model: run_repos uses composite PK (run_id, repo_id), where repo_id refers
// to mod_repos.id (NanoID(8)).
type RunRepoResponse struct {
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.ModRepoID `json:"repo_id"`
	RepoURL    string                `json:"repo_url"`
	BaseRef    string                `json:"base_ref"`
	TargetRef  string                `json:"target_ref"`
	Status     store.RunRepoStatus   `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

// runRepoToResponse converts a store.RunRepo to a RunRepoResponse.
// Wraps raw store strings in domain types for type-safe API output.
func runRepoToResponse(rr store.RunRepo, repoURL string) RunRepoResponse {
	resp := RunRepoResponse{
		RunID:     rr.RunID,
		RepoID:    rr.RepoID,
		RepoURL:   repoURL,
		BaseRef:   rr.RepoBaseRef,
		TargetRef: rr.RepoTargetRef,
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
