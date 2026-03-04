package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
	RepoID     domaintypes.RepoID `json:"repo_id"`
	RepoURL    string             `json:"repo_url"`
	LastRunAt  *time.Time         `json:"last_run_at,omitempty"`
	LastStatus *string            `json:"last_status,omitempty"`
}

// RepoRunSummary represents a run for a specific repository.
// Used in the GET /v1/repos/{repo_id}/runs response.
type RepoRunSummary struct {
	RunID      domaintypes.RunID         `json:"run_id"`
	MigID      domaintypes.MigID         `json:"mig_id"`
	RunStatus  domaintypes.RunStatus     `json:"run_status"`
	RepoStatus domaintypes.RunRepoStatus `json:"repo_status"`
	BaseRef    string                    `json:"base_ref"`
	TargetRef  string                    `json:"target_ref"`
	Attempt    int32                     `json:"attempt"`
	StartedAt  *time.Time                `json:"started_at,omitempty"`
	FinishedAt *time.Time                `json:"finished_at,omitempty"`
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
			httpErr(w, http.StatusInternalServerError, "failed to list repos: %v", err)
			slog.Error("list repos: fetch failed", "err", err, "filter", filter)
			return
		}

		// Convert store rows to API response format.
		summaries := make([]RepoSummary, 0, len(repos))
		for _, repo := range repos {
			summary := RepoSummary{
				RepoID:  repo.RepoID,
				RepoURL: repo.RepoUrl,
			}
			// Include last run timing if available.
			if repo.LastRunAt.Valid {
				t := repo.LastRunAt.Time
				summary.LastRunAt = &t
			}
			// Include last status if the row has a valid status.
			if repo.LastStatus != nil {
				switch v := repo.LastStatus.(type) {
				case string:
					if v != "" {
						summary.LastStatus = &v
					}
				case []byte:
					if s := string(v); s != "" {
						summary.LastStatus = &s
					}
				}
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
// GET /v1/repos/{repo_id}/runs — Returns runs associated with the repository ID.
// Path parameters:
//   - repo_id: repository identifier (mig_repos.id, NanoID string)
//
// Query parameters:
//   - limit: max number of runs to return (default 50, max 100)
//   - offset: number of runs to skip (default 0)
func listRunsForRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID, err := parseParam[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse pagination parameters with defaults.
		limit := int32(50)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 32)
			if err != nil || parsed < 1 {
				httpErr(w, http.StatusBadRequest, "invalid limit parameter")
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
				httpErr(w, http.StatusBadRequest, "invalid offset parameter")
				return
			}
			offset = int32(parsed)
		}

		// Fetch runs for this repository from the store.
		runs, err := st.ListRunsForRepo(r.Context(), store.ListRunsForRepoParams{
			RepoID: repoID,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list runs for repo: %v", err)
			slog.Error("list runs for repo: fetch failed", "err", err, "repo_id", repoID)
			return
		}

		// Convert store rows to API response format.
		summaries := make([]RepoRunSummary, 0, len(runs))
		for _, run := range runs {
			summary := RepoRunSummary{
				RunID:      run.RunID,
				MigID:      run.MigID,
				RunStatus:  run.RunStatus,
				RepoStatus: run.RepoStatus,
				BaseRef:    run.RepoBaseRef,
				TargetRef:  run.RepoTargetRef,
				Attempt:    run.Attempt,
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

func asNullableJSON(raw []byte) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid json payload")
	}
	cloned := append([]byte(nil), raw...)
	return json.RawMessage(cloned), nil
}
