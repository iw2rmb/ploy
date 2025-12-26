package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Repo-centric API handlers provide endpoints for listing repositories and viewing
// runs for a given repository. These complement the run-centric batch endpoints
// by offering an alternative navigation model: start from a repository URL, then
// drill down to see its run history.
//
// Endpoints:
//   - GET /v1/repos — list distinct repositories with optional ?contains= filter
//   - GET /v1/repos/{repo_id}/runs — list runs for a specific repository

// -------------------------------------------------------------------------
// Response types for repo-centric endpoints
// -------------------------------------------------------------------------

// RepoSummary represents a repository with its last run metadata.
// Used in the GET /v1/repos response to show known repositories.
type RepoSummary struct {
	RepoURL    string     `json:"repo_url"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastStatus *string    `json:"last_status,omitempty"`
}

// RepoRunSummary represents a run for a specific repository.
// Used in the GET /v1/repos/{repo_id}/runs response.
type RepoRunSummary struct {
	RunID      domaintypes.RunID `json:"run_id"`
	Name       *string           `json:"name,omitempty"`
	RunStatus  string            `json:"run_status"`
	RepoStatus string            `json:"repo_status"`
	BaseRef    string            `json:"base_ref"`
	TargetRef  string            `json:"target_ref"`
	Attempt    int32             `json:"attempt"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	// ExecutionRunID is the child execution run id (KSUID-backed string) for this repo
	// within the batch. It links to the Mods run that was created to process this repo.
	// Used by `ploy mod run pull` to fetch diffs and status for the specific repo execution.
	ExecutionRunID *string `json:"execution_run_id,omitempty"`
}

// -------------------------------------------------------------------------
// Handler implementations
// -------------------------------------------------------------------------

// listReposHandler returns an HTTP handler that lists distinct repositories.
// GET /v1/repos — Returns a list of repo summaries with optional filtering.
// Query parameters:
//   - contains: substring filter for repo_url (e.g., ?contains=org/project)
func listReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse optional filter parameter for substring matching.
		filter := r.URL.Query().Get("contains")

		// Fetch distinct repos from the store.
		// The store query handles NULL/empty filter internally to return all repos.
		repos, err := st.ListDistinctRepos(r.Context(), filter)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list repos: %v", err), http.StatusInternalServerError)
			slog.Error("list repos: fetch failed", "err", err, "filter", filter)
			return
		}

		// Convert store rows to API response format.
		summaries := make([]RepoSummary, 0, len(repos))
		for _, repo := range repos {
			summary := RepoSummary{
				RepoURL: repo.RepoUrl,
			}
			// Include last run timing if available.
			if repo.LastRunAt.Valid {
				t := repo.LastRunAt.Time
				summary.LastRunAt = &t
			}
			// Include last status if the row has a valid status.
			if repo.LastStatus != "" {
				s := string(repo.LastStatus)
				summary.LastStatus = &s
			}
			summaries = append(summaries, summary)
		}

		// Build and return response.
		resp := struct {
			Repos []RepoSummary `json:"repos"`
		}{
			Repos: summaries,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list repos: encode response failed", "err", err)
		}
	}
}

// listRunsForRepoHandler returns an HTTP handler that lists runs for a given repository.
// GET /v1/repos/{repo_id}/runs — Returns runs associated with the repo_url.
// Path parameters:
//   - repo_id: URL-encoded repository URL (e.g., https%3A%2F%2Fgithub.com%2Forg%2Frepo.git)
//
// Query parameters:
//   - limit: max number of runs to return (default 50, max 100)
//   - offset: number of runs to skip (default 0)
func listRunsForRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the repo ID (URL-encoded repo_url) from the path.
		repoIDEncoded, err := requiredPathParam(r, "repo_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// URL-decode the repo_id to get the original repo_url.
		repoURL, err := url.PathUnescape(repoIDEncoded)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid repo_id encoding: %v", err), http.StatusBadRequest)
			return
		}

		// Validate that the decoded URL is a valid RepoURL according to domain rules.
		if repoURL == "" {
			http.Error(w, "repo_id cannot be empty", http.StatusBadRequest)
			return
		}
		if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
			http.Error(w, "repo_id must be a valid repository URL", http.StatusBadRequest)
			return
		}

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

		// Fetch runs for this repository from the store.
		runs, err := st.ListRunsForRepo(r.Context(), store.ListRunsForRepoParams{
			RepoUrl: repoURL,
			Lim:     limit,
			Off:     offset,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list runs for repo: %v", err), http.StatusInternalServerError)
			slog.Error("list runs for repo: fetch failed", "err", err, "repo_url", repoURL)
			return
		}

		// Convert store rows to API response format.
		summaries := make([]RepoRunSummary, 0, len(runs))
		for _, run := range runs {
			summary := RepoRunSummary{
				RunID:          run.RunID,
				Name:           run.Name,
				RunStatus:      string(run.RunStatus),
				RepoStatus:     string(run.RepoStatus),
				BaseRef:        run.BaseRef,
				TargetRef:      run.TargetRef,
				Attempt:        run.Attempt,
				ExecutionRunID: run.ExecutionRunID, // Nullable; populated when execution started.
			}
			if run.StartedAt.Valid {
				t := run.StartedAt.Time
				summary.StartedAt = &t
			}
			if run.FinishedAt.Valid {
				t := run.FinishedAt.Time
				summary.FinishedAt = &t
			}
			summaries = append(summaries, summary)
		}

		// Build and return response.
		resp := struct {
			Runs []RepoRunSummary `json:"runs"`
		}{
			Runs: summaries,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list runs for repo: encode response failed", "err", err)
		}
	}
}
