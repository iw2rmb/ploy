package handlers

import (
	"log/slog"
	"net/http"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// RepoSummary is returned by GET /v1/repos.
type RepoSummary struct {
	RepoID     domaintypes.RepoID `json:"repo_id"`
	RepoURL    string             `json:"repo_url"`
	LastRunAt  *time.Time         `json:"last_run_at,omitempty"`
	LastStatus *string            `json:"last_status,omitempty"`
}

// RepoRunSummary is returned by GET /v1/repos/{repo_id}/runs.
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

// listReposHandler handles GET /v1/repos.
func listReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("contains")

		repos, err := st.ListDistinctRepos(r.Context(), filter)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list repos: %v", err)
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
			if repo.LastRunAt.Valid {
				t := repo.LastRunAt.Time
				summary.LastRunAt = &t
			}
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

		resp := struct {
			Repos []RepoSummary `json:"repos"`
		}{
			Repos: summaries,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// listRunsForRepoHandler handles GET /v1/repos/{repo_id}/runs.
func listRunsForRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID, err := parseRequiredPathID[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		limit, offset, err := parsePagination(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		runs, err := st.ListRunsForRepo(r.Context(), store.ListRunsForRepoParams{
			RepoID: repoID,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list runs for repo: %v", err)
			slog.Error("list runs for repo: fetch failed", "err", err, "repo_id", repoID)
			return
		}

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

		resp := struct {
			Runs []RepoRunSummary `json:"runs"`
		}{
			Runs: summaries,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
