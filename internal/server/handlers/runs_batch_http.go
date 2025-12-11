package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// NOTE: This file uses KSUID-backed string IDs for runs and NanoID(8)-backed string IDs for run_repos.
// Run IDs are KSUID strings (27 chars); run_repo IDs are NanoID strings (8 chars).
// Both are treated as opaque strings; no UUID parsing is performed.

// Batch run handlers implement HTTP endpoints for listing, inspecting, and stopping
// batched mod runs. These handlers build on the `runs` table and aggregate per-repo
// status from `run_repos` to provide batch-level lifecycle management.
//
// The package also provides BatchRepoStarter, which implements the batchscheduler.RepoStarter
// interface for background processing of pending repos.

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
		summaries := make([]RunSummary, 0, len(runs))
		for _, run := range runs {
			summary := runToSummary(run)

			// Fetch repo counts for this run to provide batch-level aggregates.
			// run.ID is now a string (KSUID).
			counts, err := getRunRepoCounts(r.Context(), st, domaintypes.RunID(run.ID))
			if err != nil {
				// Log but continue — repo counts are optional enhancement.
				slog.Warn("list runs: failed to fetch repo counts", "run_id", run.ID, "err", err)
			} else if counts.Total > 0 {
				summary.Counts = counts
			}

			summaries = append(summaries, summary)
		}

		// Return response.
		resp := struct {
			Runs []RunSummary `json:"runs"`
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
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Fetch the run using string ID directly (no UUID parsing needed).
		run, err := st.GetRun(r.Context(), runIDStr)
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
		// run.ID is now a string (KSUID).
		counts, err := getRunRepoCounts(r.Context(), st, domaintypes.RunID(run.ID))
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
//
// Run IDs are KSUID strings; run_repo IDs are NanoID strings. Both are treated as opaque.
func stopRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Fetch the run using string ID directly (no UUID parsing needed).
		run, err := st.GetRun(r.Context(), runIDStr)
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
			counts, _ := getRunRepoCounts(r.Context(), st, domaintypes.RunID(run.ID))
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
			ID:         runIDStr,
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
		repos, err := st.ListRunReposByRun(r.Context(), runIDStr)
		if err != nil {
			// Log but continue — the run is already marked as canceled.
			slog.Warn("stop run: failed to list run repos", "run_id", runIDStr, "err", err)
		} else {
			for _, repo := range repos {
				// Only cancel repos that are still pending.
				if repo.Status == store.RunRepoStatusPending {
					// repo.ID is now a string (NanoID); use directly.
					err := st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
						ID:     repo.ID,
						Status: store.RunRepoStatusCancelled,
					})
					if err != nil {
						slog.Warn("stop run: failed to cancel repo", "run_id", runIDStr, "repo_id", repo.ID, "err", err)
					}
				}
			}
		}

		slog.Info("run stopped", "run_id", runIDStr)

		// Re-fetch the run to get updated state.
		run, err = st.GetRun(r.Context(), runIDStr)
		if err != nil {
			// The run was just updated, so this should not fail. Log and return the pre-updated summary.
			slog.Error("stop run: re-fetch failed", "run_id", runIDStr, "err", err)
		}

		// Build and return the updated summary.
		summary := runToSummary(run)
		counts, _ := getRunRepoCounts(r.Context(), st, domaintypes.RunID(run.ID))
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

// addRunRepoHandler returns an HTTP handler that adds a repo to a batch run.
// POST /v1/runs/{id}/repos — Body {repo_url, base_ref, target_ref}.
// Creates a run_repos row with status=pending using a NanoID(8) for the repo ID.
// Returns 201 on success with the created repo entry.
//
// Run IDs are KSUID strings; run_repo IDs are NanoID strings generated via NewRunRepoID().
func addRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Verify the run exists before adding a repo.
		run, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: get run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Reject adding repos to terminal runs — the batch is already complete.
		if isTerminalRunStatus(run.Status) {
			http.Error(w, "cannot add repos to a terminal run", http.StatusConflict)
			return
		}

		// Decode request body with domain types for VCS fields.
		// JSON unmarshaling will automatically normalize values; we validate explicitly.
		var req struct {
			RepoURL domaintypes.RepoURL `json:"repo_url"`
			BaseRef domaintypes.GitRef  `json:"base_ref"`
			// TargetRef is optional; when omitted, downstream MR creation derives a default.
			TargetRef *domaintypes.GitRef `json:"target_ref,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		if err := req.RepoURL.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
			return
		}
		if req.TargetRef != nil {
			if err := req.TargetRef.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Create the run_repo entry with status=pending.
		// Generate a NanoID(8) for the repo ID using the ID helper.
		targetRef := ""
		if req.TargetRef != nil {
			targetRef = req.TargetRef.String()
		}

		repoID := domaintypes.NewRunRepoID()
		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			ID:        string(repoID),
			RunID:     domaintypes.RunID(runIDStr),
			RepoUrl:   req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: targetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run repo: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: create failed", "run_id", runIDStr, "repo_url", req.RepoURL, "err", err)
			return
		}

		slog.Info("run repo added",
			"run_id", runIDStr,
			"repo_id", runRepo.ID, // NanoID string; use directly.
			"repo_url", req.RepoURL,
		)

		// Return the created repo entry.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(runRepoToResponse(runRepo)); err != nil {
			slog.Error("add run repo: encode response failed", "err", err)
		}
	}
}

// listRunReposHandler returns an HTTP handler that lists repos within a batch run.
// GET /v1/runs/{id}/repos — Returns the list of repos with status, attempt, timing fields.
//
// Run IDs are KSUID strings; treated as opaque identifiers.
func listRunReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Verify the run exists before listing repos.
		_, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("list run repos: get run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Fetch all repos for this run.
		repos, err := st.ListRunReposByRun(r.Context(), runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("list run repos: list failed", "run_id", runIDStr, "err", err)
			return
		}

		// Build response.
		reposResp := make([]RunRepoResponse, 0, len(repos))
		for _, rr := range repos {
			reposResp = append(reposResp, runRepoToResponse(rr))
		}

		resp := struct {
			Repos []RunRepoResponse `json:"repos"`
		}{
			Repos: reposResp,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list run repos: encode response failed", "err", err)
		}
	}
}

// deleteRunRepoHandler returns an HTTP handler that removes/cancels a repo from a batch.
// DELETE /v1/runs/{id}/repos/{repo_id} — Marks pending repos as skipped, running repos as cancelled.
// Returns 200 on success with the updated repo entry.
//
// Run IDs are KSUID strings; run_repo IDs are NanoID strings. Both are treated as opaque.
func deleteRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse the repo ID from the URL path parameter.
		// Repo IDs are NanoID strings (8 chars); treated as opaque identifiers.
		repoIDStr := strings.TrimSpace(r.PathValue("repo_id"))
		if repoIDStr == "" {
			http.Error(w, "repo_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Verify the run exists.
		_, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("delete run repo: get run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Fetch the repo entry using string ID directly.
		runRepo, err := st.GetRunRepo(r.Context(), repoIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repo not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
			slog.Error("delete run repo: get repo failed", "repo_id", repoIDStr, "err", err)
			return
		}

		// Verify the repo belongs to this run by comparing string run IDs.
		if runRepo.RunID.String() != runIDStr {
			http.Error(w, "repo does not belong to this run", http.StatusNotFound)
			return
		}

		// Determine the new status based on current status.
		// Pending → Skipped (never started, user removed it).
		// Running → Cancelled (will need execution to stop).
		// Terminal statuses → No change (idempotent).
		var newStatus store.RunRepoStatus
		switch runRepo.Status {
		case store.RunRepoStatusPending:
			newStatus = store.RunRepoStatusSkipped
		case store.RunRepoStatusRunning:
			newStatus = store.RunRepoStatusCancelled
		default:
			// Already terminal — return current state (idempotent).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(runRepoToResponse(runRepo)); err != nil {
				slog.Error("delete run repo: encode response failed", "err", err)
			}
			return
		}

		// Update the repo status using string ID.
		err = st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
			ID:     repoIDStr,
			Status: newStatus,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update repo status: %v", err), http.StatusInternalServerError)
			slog.Error("delete run repo: update status failed", "repo_id", repoIDStr, "err", err)
			return
		}

		slog.Info("run repo deleted",
			"run_id", runIDStr,
			"repo_id", repoIDStr,
			"old_status", runRepo.Status,
			"new_status", newStatus,
		)

		// Re-fetch to get updated timestamps.
		runRepo, err = st.GetRunRepo(r.Context(), repoIDStr)
		if err != nil {
			// Log but return success — the status was updated.
			slog.Warn("delete run repo: re-fetch failed", "repo_id", repoIDStr, "err", err)
			runRepo.Status = newStatus // Use the intended status in response.
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(runRepoToResponse(runRepo)); err != nil {
			slog.Error("delete run repo: encode response failed", "err", err)
		}
	}
}

// restartRunRepoHandler returns an HTTP handler that restarts a repo within a batch.
// POST /v1/runs/{id}/repos/{repo_id}/restart — Resets status to pending, increments attempt.
// Optionally updates base_ref/target_ref from request body.
// Returns 200 on success with the updated repo entry.
//
// Run IDs are KSUID strings; run_repo IDs are NanoID strings. Both are treated as opaque.
func restartRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse the repo ID from the URL path parameter.
		// Repo IDs are NanoID strings (8 chars); treated as opaque identifiers.
		repoIDStr := strings.TrimSpace(r.PathValue("repo_id"))
		if repoIDStr == "" {
			http.Error(w, "repo_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Verify the run exists and is not terminal.
		run, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("restart run repo: get run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Reject restarting repos in terminal runs — the batch is already complete.
		if isTerminalRunStatus(run.Status) {
			http.Error(w, "cannot restart repos in a terminal run", http.StatusConflict)
			return
		}

		// Fetch the repo entry using string ID directly.
		runRepo, err := st.GetRunRepo(r.Context(), repoIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repo not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
			slog.Error("restart run repo: get repo failed", "repo_id", repoIDStr, "err", err)
			return
		}

		// Verify the repo belongs to this run by comparing string run IDs.
		if runRepo.RunID.String() != runIDStr {
			http.Error(w, "repo does not belong to this run", http.StatusNotFound)
			return
		}

		// Only allow restart from terminal states (succeeded, failed, skipped, cancelled).
		// Pending or running repos cannot be restarted — they haven't completed yet.
		if !isTerminalRunRepoStatus(runRepo.Status) {
			http.Error(w, "can only restart repos in terminal state", http.StatusConflict)
			return
		}

		// Decode optional request body for ref updates.
		// Empty body is allowed — just restart with existing refs.
		var req struct {
			BaseRef   *domaintypes.GitRef `json:"base_ref,omitempty"`
			TargetRef *domaintypes.GitRef `json:"target_ref,omitempty"`
		}
		// Only decode if body is present (Content-Length > 0 or chunked).
		if r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") == "chunked" {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
				return
			}

			// Validate provided refs when present.
			if req.BaseRef != nil {
				if err := req.BaseRef.Validate(); err != nil {
					http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
					return
				}
			}
			if req.TargetRef != nil {
				if err := req.TargetRef.Validate(); err != nil {
					http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
					return
				}
			}
		}

		// Increment attempt and reset status to pending.
		// This uses IncrementRunRepoAttempt which also clears timing fields.
		err = st.IncrementRunRepoAttempt(r.Context(), repoIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to restart repo: %v", err), http.StatusInternalServerError)
			slog.Error("restart run repo: increment attempt failed", "repo_id", repoIDStr, "err", err)
			return
		}

		// If base_ref/target_ref updates are provided, persist them.
		if req.BaseRef != nil || req.TargetRef != nil {
			newBaseRef := runRepo.BaseRef
			if req.BaseRef != nil {
				newBaseRef = req.BaseRef.String()
			}
			newTargetRef := runRepo.TargetRef
			if req.TargetRef != nil {
				newTargetRef = req.TargetRef.String()
			}

			if err := st.UpdateRunRepoRefs(r.Context(), store.UpdateRunRepoRefsParams{
				ID:        repoIDStr,
				BaseRef:   newBaseRef,
				TargetRef: newTargetRef,
			}); err != nil {
				http.Error(w, fmt.Sprintf("failed to update repo refs: %v", err), http.StatusInternalServerError)
				slog.Error("restart run repo: update refs failed", "repo_id", repoIDStr, "err", err)
				return
			}
		}

		slog.Info("run repo restarted",
			"run_id", runIDStr,
			"repo_id", repoIDStr,
			"old_status", runRepo.Status,
			"old_attempt", runRepo.Attempt,
		)

		// Re-fetch to get updated state.
		runRepo, err = st.GetRunRepo(r.Context(), repoIDStr)
		if err != nil {
			// Log but return success — the restart was applied.
			slog.Warn("restart run repo: re-fetch failed", "repo_id", repoIDStr, "err", err)
			// Simulate the expected state in response.
			runRepo.Status = store.RunRepoStatusPending
			runRepo.Attempt++
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(runRepoToResponse(runRepo)); err != nil {
			slog.Error("restart run repo: encode response failed", "err", err)
		}
	}
}

// -------------------------------------------------------------------------
// Batch execution handlers — connect RunRepo entries to execution runs.
// -------------------------------------------------------------------------

// StartRunResponse contains the result of starting a batch run.
// Uses domain type RunID for type-safe run identification.
type StartRunResponse struct {
	RunID       domaintypes.RunID `json:"run_id"`
	Started     int               `json:"started"`      // Number of repos that started execution.
	AlreadyDone int               `json:"already_done"` // Number of repos already in terminal state.
	Pending     int               `json:"pending"`      // Number of repos still pending (if any).
}

// startRunHandler returns an HTTP handler that starts execution for pending repos in a batch.
// POST /v1/runs/{id}/start — Creates child execution runs for each pending repo and creates jobs.
// Returns 200 on success with counts of started, already done, and remaining pending repos.
//
// Run IDs are KSUID strings; run_repo IDs are NanoID strings. Both are treated as opaque.
func startRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Fetch the batch run to get shared spec and verify it exists.
		batchRun, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("start run: fetch failed", "run_id", runIDStr, "err", err)
			return
		}

		// Reject starting execution for terminal runs.
		if isTerminalRunStatus(batchRun.Status) {
			http.Error(w, "cannot start repos in a terminal run", http.StatusConflict)
			return
		}

		// Fetch all repos for this batch to get counts.
		allRepos, err := st.ListRunReposByRun(r.Context(), runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("start run: list repos failed", "run_id", runIDStr, "err", err)
			return
		}

		// Count repos by status.
		var alreadyDone, stillPending int
		for _, repo := range allRepos {
			if isTerminalRunRepoStatus(repo.Status) {
				alreadyDone++
			} else if repo.Status == store.RunRepoStatusPending {
				stillPending++
			}
			// Running repos are counted as in-progress, not started by this call.
		}

		// Fetch pending repos that need to start execution.
		pendingRepos, err := st.ListPendingRunReposByRun(r.Context(), runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list pending repos: %v", err), http.StatusInternalServerError)
			slog.Error("start run: list pending repos failed", "run_id", runIDStr, "err", err)
			return
		}

		// If no pending repos, return early with current counts.
		if len(pendingRepos) == 0 {
			resp := StartRunResponse{
				RunID:       domaintypes.RunID(runIDStr),
				Started:     0,
				AlreadyDone: alreadyDone,
				Pending:     0,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				slog.Error("start run: encode response failed", "err", err)
			}
			return
		}

		// Start execution for each pending repo by creating a child run and jobs.
		var started int
		for _, repo := range pendingRepos {
			// Create child execution run for this repo.
			// The child run inherits the spec and commit_sha from the batch but has
			// repo-specific URL/refs.
			childRunID := domaintypes.NewRunID()
			childRun, err := st.CreateRun(r.Context(), store.CreateRunParams{
				ID:        string(childRunID),
				Name:      nil, // Child runs don't need a name; batch name is on parent.
				RepoUrl:   repo.RepoUrl,
				Spec:      batchRun.Spec,
				CreatedBy: batchRun.CreatedBy,
				Status:    store.RunStatusQueued,
				BaseRef:   repo.BaseRef,
				TargetRef: repo.TargetRef,
				CommitSha: batchRun.CommitSha,
			})
			if err != nil {
				slog.Error("start run: create child run failed",
					"run_id", runIDStr,
					"repo_id", repo.ID, // NanoID string; use directly.
					"repo_url", repo.RepoUrl,
					"err", err,
				)
				continue // Skip this repo but try others.
			}

			// Create jobs from the batch spec for this child run.
			if err := createJobsFromSpec(r.Context(), st, domaintypes.RunID(childRun.ID), batchRun.Spec); err != nil {
				slog.Error("start run: create jobs failed",
					"run_id", runIDStr,
					"child_run_id", childRun.ID, // KSUID string; use directly.
					"repo_url", repo.RepoUrl,
					"err", err,
				)
				// Clean up the orphaned child run.
				_ = st.DeleteRun(r.Context(), childRun.ID)
				continue // Skip this repo but try others.
			}

			// Link the repo entry to its child execution run and mark as running.
			// Both IDs are now strings (NanoID for repo, KSUID for child run).
			err = st.SetRunRepoExecutionRun(r.Context(), store.SetRunRepoExecutionRunParams{
				ID:             repo.ID,
				ExecutionRunID: &childRun.ID,
			})
			if err != nil {
				slog.Error("start run: link repo to child run failed",
					"run_id", runIDStr,
					"repo_id", repo.ID, // NanoID string; use directly.
					"child_run_id", childRun.ID, // KSUID string; use directly.
					"err", err,
				)
				// The child run exists but isn't linked; it will still execute.
				// Log but count as started since jobs were created.
			}

			started++
			slog.Info("run repo execution started",
				"run_id", runIDStr,
				"repo_id", repo.ID, // NanoID string; use directly.
				"child_run_id", childRun.ID, // KSUID string; use directly.
				"repo_url", repo.RepoUrl,
			)
		}

		// Update batch run status to running if we started at least one repo.
		if started > 0 && batchRun.Status == store.RunStatusQueued {
			if err := st.AckRunStart(r.Context(), runIDStr); err != nil {
				slog.Warn("start run: failed to update batch status to running", "run_id", runIDStr, "err", err)
			}
		}

		// Build response.
		resp := StartRunResponse{
			RunID:       domaintypes.RunID(runIDStr),
			Started:     started,
			AlreadyDone: alreadyDone,
			Pending:     stillPending - started, // Subtract started from pending count.
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("start run: encode response failed", "err", err)
		}
	}
}
