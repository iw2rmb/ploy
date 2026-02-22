package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// --- Batch types and helpers (from runs_batch_types.go) ---

// NOTE: Run IDs in this file are KSUID-backed strings; run_repo IDs are NanoID(8)-backed strings.
// Both are now string types in the store layer; no UUID parsing is needed.

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
func runToSummary(run store.Run) domaintypes.RunSummary {
	summary := domaintypes.RunSummary{
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
func getRunRepoCounts(ctx context.Context, st store.Store, runID domaintypes.RunID) (*domaintypes.RunRepoCounts, error) {
	rows, err := st.CountRunReposByStatus(ctx, runID)
	if err != nil {
		return nil, err
	}

	counts := &domaintypes.RunRepoCounts{}
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
func deriveBatchStatus(counts *domaintypes.RunRepoCounts) string {
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

// --- Run handlers (from runs.go) ---

// getRunTimingHandler returns an HTTP handler that retrieves timing data for a run.
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
func getRunTimingHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept id from path parameter first, then fallback to query parameter.
		// Uses domain type helpers for validation at the boundary.
		var runID domaintypes.RunID
		if idPtr, err := optionalParam[domaintypes.RunID](r, "id"); err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		} else if idPtr != nil {
			runID = *idPtr
		} else if qID, err := optionalQuery[domaintypes.RunID](r, "id"); err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		} else if qID != nil {
			runID = *qID
		} else {
			httpErr(w, http.StatusBadRequest, "id query parameter is required")
			return
		}
		timing, err := st.GetRunTiming(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run timing: %v", err)
			slog.Error("get run timing: database error", "run_id", runID, "err", err)
			return
		}

		// Build response. timing.ID is now a string (KSUID).
		resp := struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		}{
			ID:      timing.ID.String(),
			QueueMs: timing.QueueMs,
			RunMs:   timing.RunMs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get run timing: encode response failed", "err", err)
		}

		slog.Info("run timing retrieved", "run_id", resp.ID, "queue_ms", resp.QueueMs, "run_ms", resp.RunMs)
	}
}

// deleteRunHandler returns an HTTP handler that deletes a run by id.
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
func deleteRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract id from path parameter using domain type helper.
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check if the run exists before attempting to delete.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check run: %v", err)
			slog.Error("delete run: check failed", "run_id", runID, "err", err)
			return
		}

		// Delete the run using string ID directly.
		err = st.DeleteRun(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to delete run: %v", err)
			slog.Error("delete run: database error", "run_id", runID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run deleted", "run_id", runID)
	}
}

// --- Run list handlers (from runs_list.go) ---

func listRunsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := int32(50)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 32)
			if err != nil || parsed < 1 {
				httpErr(w, http.StatusBadRequest, "invalid limit parameter")
				return
			}
			limit = int32(parsed)
			if limit > 100 {
				limit = 100
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			parsed, err := strconv.ParseInt(o, 10, 32)
			if err != nil || parsed < 0 {
				httpErr(w, http.StatusBadRequest, "invalid offset parameter")
				return
			}
			offset = int32(parsed)
		}

		runs, err := st.ListRuns(r.Context(), store.ListRunsParams{Limit: limit, Offset: offset})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list runs: %v", err)
			slog.Error("list runs: fetch failed", "err", err)
			return
		}

		summaries := make([]domaintypes.RunSummary, 0, len(runs))
		for _, run := range runs {
			summaries = append(summaries, runToSummary(run))
		}

		resp := struct {
			Runs []domaintypes.RunSummary `json:"runs"`
		}{Runs: summaries}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("get run: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		summary := runToSummary(run)
		if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	}
}
