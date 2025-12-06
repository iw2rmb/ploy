package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// Batch run handlers implement HTTP endpoints for listing, inspecting, and stopping
// batched mod runs. These handlers build on the `runs` table and aggregate per-repo
// status from `run_repos` to provide batch-level lifecycle management.

// RunBatchSummary represents a run with aggregated repo status counts.
// Used for list and detail responses.
type RunBatchSummary struct {
	ID         string          `json:"id"`
	Name       *string         `json:"name,omitempty"`
	Status     store.RunStatus `json:"status"`
	RepoURL    string          `json:"repo_url"`
	BaseRef    string          `json:"base_ref"`
	TargetRef  string          `json:"target_ref"`
	CreatedBy  *string         `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Counts     *RunRepoCounts  `json:"repo_counts,omitempty"`
}

// RunRepoCounts aggregates the count of repos by status within a batch.
type RunRepoCounts struct {
	Total     int32 `json:"total"`
	Pending   int32 `json:"pending"`
	Running   int32 `json:"running"`
	Succeeded int32 `json:"succeeded"`
	Failed    int32 `json:"failed"`
	Skipped   int32 `json:"skipped"`
	Cancelled int32 `json:"cancelled"`
}

// listRunsHandler returns an HTTP handler that lists runs with pagination.
// GET /v1/runs — Returns a list of run summaries ordered by creation time descending.
// Query parameters:
//   - limit: max number of runs to return (default 50, max 100)
//   - offset: number of runs to skip (default 0)
func listRunsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse pagination parameters with defaults.
		limit := int32(50)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 32)
			if err != nil || parsed < 1 {
				http.Error(w, "invalid limit parameter", http.StatusBadRequest)
				return
			}
			limit = int32(parsed)
			// Cap at 100 to avoid excessive load.
			if limit > 100 {
				limit = 100
			}
		}

		if o := r.URL.Query().Get("offset"); o != "" {
			parsed, err := strconv.ParseInt(o, 10, 32)
			if err != nil || parsed < 0 {
				http.Error(w, "invalid offset parameter", http.StatusBadRequest)
				return
			}
			offset = int32(parsed)
		}

		// Fetch runs from the store.
		runs, err := st.ListRuns(r.Context(), store.ListRunsParams{
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list runs: %v", err), http.StatusInternalServerError)
			slog.Error("list runs: fetch failed", "err", err)
			return
		}

		// Build response summaries with optional repo counts.
		// For efficiency, we fetch repo counts per run to include batch aggregate data.
		summaries := make([]RunBatchSummary, 0, len(runs))
		for _, run := range runs {
			summary := runToSummary(run)

			// Fetch repo counts for this run to provide batch-level aggregates.
			counts, err := getRunRepoCounts(r.Context(), st, run.ID)
			if err != nil {
				// Log but continue — repo counts are optional enhancement.
				slog.Warn("list runs: failed to fetch repo counts", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			} else if counts.Total > 0 {
				summary.Counts = counts
			}

			summaries = append(summaries, summary)
		}

		// Return response.
		resp := struct {
			Runs []RunBatchSummary `json:"runs"`
		}{
			Runs: summaries,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list runs: encode response failed", "err", err)
		}
	}
}

// getRunHandler returns an HTTP handler that fetches a single run by ID.
// GET /v1/runs/{id} — Returns detailed run summary including batch-level status and repo counts.
func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		runIDStr := r.PathValue("id")
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse UUID.
		runID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run id: %v", err), http.StatusBadRequest)
			return
		}

		// Convert to pgtype.UUID.
		pgID := pgtype.UUID{
			Bytes: runID,
			Valid: true,
		}

		// Fetch the run.
		run, err := st.GetRun(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("get run: fetch failed", "run_id", runIDStr, "err", err)
			return
		}

		// Build summary with repo counts.
		summary := runToSummary(run)

		// Fetch repo counts to provide batch-level aggregates.
		counts, err := getRunRepoCounts(r.Context(), st, run.ID)
		if err != nil {
			slog.Warn("get run: failed to fetch repo counts", "run_id", runIDStr, "err", err)
		} else if counts.Total > 0 {
			summary.Counts = counts
		}

		// Return response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("get run: encode response failed", "err", err)
		}
	}
}

// stopRunHandler returns an HTTP handler that stops a batched run.
// POST /v1/runs/{id}/stop — Marks the run as canceled and cancels all pending run_repos.
// Returns 200 on success with the updated run summary.
func stopRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		runIDStr := r.PathValue("id")
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse UUID.
		runID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run id: %v", err), http.StatusBadRequest)
			return
		}

		// Convert to pgtype.UUID.
		pgID := pgtype.UUID{
			Bytes: runID,
			Valid: true,
		}

		// Fetch the run to verify it exists and check current status.
		run, err := st.GetRun(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("stop run: fetch failed", "run_id", runIDStr, "err", err)
			return
		}

		// Check if the run is already in a terminal state.
		if isTerminalRunStatus(run.Status) {
			// Run is already terminal — return current state without error.
			// This makes the stop operation idempotent.
			summary := runToSummary(run)
			counts, _ := getRunRepoCounts(r.Context(), st, run.ID)
			if counts != nil && counts.Total > 0 {
				summary.Counts = counts
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(summary); err != nil {
				slog.Error("stop run: encode response failed", "err", err)
			}
			return
		}

		// Update the run status to canceled.
		err = st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{
			ID:         pgID,
			Status:     store.RunStatusCanceled,
			FinishedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
			slog.Error("stop run: update status failed", "run_id", runIDStr, "err", err)
			return
		}

		// Cancel all pending run_repos entries for this batch.
		// Fetch and update each pending repo to 'cancelled'.
		repos, err := st.ListRunReposByRun(r.Context(), pgID)
		if err != nil {
			// Log but continue — the run is already marked as canceled.
			slog.Warn("stop run: failed to list run repos", "run_id", runIDStr, "err", err)
		} else {
			for _, repo := range repos {
				// Only cancel repos that are still pending.
				if repo.Status == store.RunRepoStatusPending {
					err := st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
						ID:     repo.ID,
						Status: store.RunRepoStatusCancelled,
					})
					if err != nil {
						slog.Warn("stop run: failed to cancel repo", "run_id", runIDStr, "repo_id", uuid.UUID(repo.ID.Bytes).String(), "err", err)
					}
				}
			}
		}

		slog.Info("run stopped", "run_id", runIDStr)

		// Re-fetch the run to get updated state.
		run, err = st.GetRun(r.Context(), pgID)
		if err != nil {
			// The run was just updated, so this should not fail. Log and return the pre-updated summary.
			slog.Error("stop run: re-fetch failed", "run_id", runIDStr, "err", err)
		}

		// Build and return the updated summary.
		summary := runToSummary(run)
		counts, _ := getRunRepoCounts(r.Context(), st, run.ID)
		if counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("stop run: encode response failed", "err", err)
		}
	}
}

// runToSummary converts a store.Run to a RunBatchSummary.
func runToSummary(run store.Run) RunBatchSummary {
	summary := RunBatchSummary{
		ID:        uuid.UUID(run.ID.Bytes).String(),
		Name:      run.Name,
		Status:    run.Status,
		RepoURL:   run.RepoUrl,
		BaseRef:   run.BaseRef,
		TargetRef: run.TargetRef,
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
func getRunRepoCounts(ctx context.Context, st store.Store, runID pgtype.UUID) (*RunRepoCounts, error) {
	rows, err := st.CountRunReposByStatus(ctx, runID)
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

	return counts, nil
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
