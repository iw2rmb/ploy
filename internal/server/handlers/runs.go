package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// NOTE: Run IDs in this file are KSUID-backed strings; repo IDs are NanoID(8)-backed strings.
// Both are now string types in the store layer; no UUID parsing is needed.

// runToSummary converts a store.Run to a RunSummary.
// Wraps raw store strings in domain types for type-safe API output.
// run.ID is now a string (KSUID), so no UUID conversion is needed.
func runToSummary(run store.Run) domaintypes.RunSummary {
	summary := domaintypes.RunSummary{
		ID:              run.ID,
		Status:          run.Status,
		MigID:           run.MigID,
		SpecID:          run.SpecID,
		RepoID:          run.RepoID,
		BaseRef:         run.RepoBaseRef,
		SourceCommitSHA: run.SourceCommitSha,
		Attempt:         run.Attempt,
		LastError:       run.LastError,
		CreatedBy:       run.CreatedBy,
		CreatedAt:       run.CreatedAt.Time,
	}

	if run.StartedAt.Valid {
		summary.StartedAt = &run.StartedAt.Time
	}
	if run.FinishedAt.Valid {
		summary.FinishedAt = &run.FinishedAt.Time
	}

	return summary
}

// getRunCounts fetches and aggregates run counts by status for the run's wave.
// runID is now a KSUID-backed domain type.
func getRunCounts(ctx context.Context, st store.Store, runID domaintypes.RunID) (*domaintypes.RunCounts, error) {
	run, err := st.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	rows, err := st.CountRunsByWaveStatus(ctx, run.WaveID)
	if err != nil {
		return nil, err
	}

	counts := &domaintypes.RunCounts{}
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case domaintypes.RunStatusQueued:
			counts.Queued = row.Count
		case domaintypes.RunStatusRunning:
			counts.Running = row.Count
		case domaintypes.RunStatusSuccess:
			counts.Success = row.Count
		case domaintypes.RunStatusFail:
			counts.Fail = row.Count
		case domaintypes.RunStatusCancelled:
			counts.Cancelled = row.Count
		}
	}

	// Derive batch-level status from repo counts.
	counts.DerivedStatus = lifecycle.DeriveBatchStatus(counts)

	return counts, nil
}

// RunResponse represents one repository run within a wave for API responses.
// Exposes repo URL, refs, attempt count, status, error, and timing fields.
// v1 model: runs stores one repository execution; repo_id refers to repos.id.
type RunResponse struct {
	RunID           domaintypes.RunID     `json:"run_id"`
	RepoID          domaintypes.RepoID    `json:"repo_id"`
	RepoURL         string                `json:"repo_url"`
	BaseRef         string                `json:"base_ref"`
	SourceCommitSHA string                `json:"source_commit_sha,omitempty"`
	Status          domaintypes.RunStatus `json:"status"`
	Attempt         int32                 `json:"attempt"`
	LastError       *string               `json:"last_error,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	StartedAt       *time.Time            `json:"started_at,omitempty"`
	FinishedAt      *time.Time            `json:"finished_at,omitempty"`
}

// runToResponse converts a store.Run to a RunResponse.
// Wraps raw store strings in domain types for type-safe API output.
func runToResponse(rr store.Run, repoURL string) RunResponse {
	resp := RunResponse{
		RunID:           rr.ID,
		RepoID:          rr.RepoID,
		RepoURL:         repoURL,
		BaseRef:         rr.RepoBaseRef,
		SourceCommitSHA: rr.SourceCommitSha,
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

func listRunsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, err := parsePagination(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		repoURL := strings.TrimSpace(r.URL.Query().Get("repo_url"))
		if repoURL != "" {
			listRunsForRepoURL(w, r, st, repoURL, limit, offset)
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

func listRunsForRepoURL(w http.ResponseWriter, r *http.Request, st store.Store, repoURL string, limit, offset int32) {
	normalizedURL := domaintypes.NormalizeRepoURL(repoURL)
	repos, err := st.ListDistinctRepos(r.Context(), repoURL)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "failed to list repos: %v", err)
		return
	}
	var matches []store.ListDistinctReposRow
	for _, repo := range repos {
		if domaintypes.NormalizeRepoURL(repo.RepoUrl) == normalizedURL {
			matches = append(matches, repo)
		}
	}
	if len(matches) == 0 {
		writeJSON(w, http.StatusOK, struct {
			Runs []domaintypes.RunSummary `json:"runs"`
		}{Runs: []domaintypes.RunSummary{}})
		return
	}
	if len(matches) > 1 {
		writeHTTPError(w, http.StatusConflict, "multiple repos match the given repo_url")
		return
	}
	repoRuns, err := st.ListRunsForRepo(r.Context(), store.ListRunsForRepoParams{
		RepoID: matches[0].RepoID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "failed to list runs for repo: %v", err)
		return
	}
	summaries := make([]domaintypes.RunSummary, 0, len(repoRuns))
	for _, run := range repoRuns {
		summary := domaintypes.RunSummary{
			ID:     run.RunID,
			Status: run.Status,
			MigID:  run.MigID,
		}
		if run.StartedAt.Valid {
			summary.StartedAt = &run.StartedAt.Time
		}
		if run.FinishedAt.Valid {
			summary.FinishedAt = &run.FinishedAt.Time
		}
		summaries = append(summaries, summary)
	}
	writeJSON(w, http.StatusOK, struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}{Runs: summaries})
}

func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run")
		if !ok {
			return
		}

		summary := runToSummary(run)
		if !run.MigID.IsZero() {
			if mig, err := st.GetMig(r.Context(), run.MigID); err == nil {
				summary.MigName = mig.Name
			}
		}
		if !run.RepoID.IsZero() {
			if repo, err := st.GetRepo(r.Context(), run.RepoID); err == nil {
				summary.RepoURL = repo.Url
			}
		}
		if counts, _ := getRunCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}

		writeJSON(w, http.StatusOK, summary)
	}
}
