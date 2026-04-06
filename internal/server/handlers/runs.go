package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// --- Batch types and helpers (from runs_batch_types.go) ---

// NOTE: Run IDs in this file are KSUID-backed strings; run_repo IDs are NanoID(8)-backed strings.
// Both are now string types in the store layer; no UUID parsing is needed.

// runToSummary converts a store.Run to a RunSummary.
// Wraps raw store strings in domain types for type-safe API output.
// run.ID is now a string (KSUID), so no UUID conversion is needed.
func runToSummary(run store.Run) domaintypes.RunSummary {
	summary := domaintypes.RunSummary{
		// run.ID is now a string (KSUID); cast directly to domain type.
		ID:        run.ID,
		Status:    run.Status,
		MigID:     run.MigID,
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
		case domaintypes.RunRepoStatusQueued:
			counts.Queued = row.Count
		case domaintypes.RunRepoStatusRunning:
			counts.Running = row.Count
		case domaintypes.RunRepoStatusSuccess:
			counts.Success = row.Count
		case domaintypes.RunRepoStatusFail:
			counts.Fail = row.Count
		case domaintypes.RunRepoStatusCancelled:
			counts.Cancelled = row.Count
		}
	}

	// Derive batch-level status from repo counts.
	counts.DerivedStatus = lifecycle.DeriveBatchStatus(counts)

	return counts, nil
}

// RunRepoResponse represents a single repo within a batch for API responses.
// Exposes repo URL, refs, attempt count, status, error, and timing fields.
// v1 model: run_repos uses composite PK (run_id, repo_id), where repo_id refers
// to mig_repos.id (NanoID(8)).
type RunRepoResponse struct {
	RunID           domaintypes.RunID         `json:"run_id"`
	RepoID          domaintypes.RepoID        `json:"repo_id"`
	RepoURL         string                    `json:"repo_url"`
	BaseRef         string                    `json:"base_ref"`
	TargetRef       string                    `json:"target_ref"`
	SourceCommitSHA string                    `json:"source_commit_sha,omitempty"`
	MROnSuccess     bool                      `json:"mr_on_success,omitempty"`
	MROnFail        bool                      `json:"mr_on_fail,omitempty"`
	Status          domaintypes.RunRepoStatus `json:"status"`
	Attempt         int32                     `json:"attempt"`
	LastError       *string                   `json:"last_error,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	StartedAt       *time.Time                `json:"started_at,omitempty"`
	FinishedAt      *time.Time                `json:"finished_at,omitempty"`
}

// runRepoToResponse converts a store.RunRepo to a RunRepoResponse.
// Wraps raw store strings in domain types for type-safe API output.
func runRepoToResponse(rr store.RunRepo, repoURL string, mrOnSuccess, mrOnFail bool) RunRepoResponse {
	resp := RunRepoResponse{
		RunID:           rr.RunID,
		RepoID:          rr.RepoID,
		RepoURL:         repoURL,
		BaseRef:         rr.RepoBaseRef,
		TargetRef:       rr.RepoTargetRef,
		SourceCommitSHA: rr.SourceCommitSha,
		MROnSuccess:     mrOnSuccess,
		MROnFail:        mrOnFail,
		Status:          rr.Status,
		Attempt:         rr.Attempt,
		LastError:       rr.LastError,
		CreatedAt:       rr.CreatedAt.Time,
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
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		} else if idPtr != nil {
			runID = *idPtr
		} else if qID, err := optionalQuery[domaintypes.RunID](r, "id"); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		} else if qID != nil {
			runID = *qID
		} else {
			writeHTTPError(w, http.StatusBadRequest, "id query parameter is required")
			return
		}
		timing, err := st.GetRunTiming(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run timing: %v", err)
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

		writeJSON(w, http.StatusOK, resp)

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
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check if the run exists before attempting to delete.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to check run: %v", err)
			slog.Error("delete run: check failed", "run_id", runID, "err", err)
			return
		}

		// Delete the run using string ID directly.
		err = st.DeleteRun(r.Context(), runID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to delete run: %v", err)
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
		limit, offset, err := parsePagination(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		runs, err := st.ListRuns(r.Context(), store.ListRunsParams{Limit: limit, Offset: offset})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list runs: %v", err)
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

		writeJSON(w, http.StatusOK, resp)
	}
}

func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("get run: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		summary := runToSummary(run)
		if !run.MigID.IsZero() {
			if mig, err := st.GetMig(r.Context(), run.MigID); err == nil {
				summary.MigName = mig.Name
			}
		}
		if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}

		writeJSON(w, http.StatusOK, summary)
	}
}
