// Package handlers implements HTTP handlers for the ploy server API.
//
// pull.go implements the "pull resolution" endpoints for fetching diffs.
// These endpoints help CLI clients resolve repo execution identifiers needed to
// pull diffs from the server.
//
// Endpoints:
//   - POST /v1/runs/{run_id}/pull — resolve repo for a specific run
//   - POST /v1/migs/{mig_id}/pull — resolve repo for a mig (last succeeded/failed)
//
// Implements pull resolution endpoints for mig and run repos.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// -------------------------------------------------------------------------
// Request/Response types for pull resolution endpoints
// -------------------------------------------------------------------------

// runPullRequest is the request body for POST /v1/runs/{run_id}/pull.
// The client provides a repo_url to resolve to execution identifiers.
type runPullRequest struct {
	RepoURL string `json:"repo_url"`
}

// modPullRequest is the request body for POST /v1/migs/{mig_id}/pull.
// The client provides a repo_url and optional mode to select which run to resolve.
type modPullRequest struct {
	RepoURL string `json:"repo_url"`
	// Mode selects which run to return:
	//   - "last-succeeded" (default): newest terminal run_repos with status=Success
	//   - "last-failed": newest terminal run_repos with status=Fail
	Mode string `json:"mode,omitempty"`
}

// pullResponse is the response for both pull resolution endpoints.
// It provides the identifiers needed to fetch diffs:
//   - run_id: the run containing the execution
//   - repo_id: the mig_repos.id for the matched repo
//   - repo_target_ref: the target ref snapshot from run_repos
type pullResponse struct {
	RunID         domaintypes.RunID  `json:"run_id"`
	RepoID        domaintypes.RepoID `json:"repo_id"`
	RepoTargetRef string             `json:"repo_target_ref"`
}

// -------------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------------

// pullRunRepoHandler resolves a repo_url to execution identifiers for a specific run.
// Endpoint: POST /v1/runs/{run_id}/pull
// Request: {repo_url}
// Response: 200 OK with {run_id, repo_id, repo_target_ref}
//
// v1 contract:
//   - Server matches the repo by joining run_repos to mig_repos by repo_id,
//     filtering by run_id, and comparing normalized repo_url.
//   - Uses domaintypes.NormalizeRepoURL for URL comparison.
//   - If no repo matches: 404 error.
//   - If multiple repos match: 409 error (ambiguous).
func pullRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run_id from path.
		runID, err := parseParam[domaintypes.RunID](r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse request body with strict validation.
		var req runPullRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_url is provided.
		if req.RepoURL == "" {
			httpErr(w, http.StatusBadRequest, "repo_url is required")
			return
		}

		// Normalize the incoming repo_url for comparison.
		normalizedURL := domaintypes.NormalizeRepoURL(req.RepoURL)

		// Verify the run exists before querying repos.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("pull run repo: get run failed", "run_id", runID, "err", err)
			return
		}

		// List all repos in this run with their URLs.
		// We need to iterate and compare normalized URLs to find matches.
		runRepos, err := st.ListRunReposWithURLByRun(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list run repos: %v", err)
			slog.Error("pull run repo: list run repos failed", "run_id", runID, "err", err)
			return
		}

		// Find matching repos by normalized URL.
		var matches []store.ListRunReposWithURLByRunRow
		for _, rr := range runRepos {
			if domaintypes.NormalizeRepoURL(rr.RepoUrl) == normalizedURL {
				matches = append(matches, rr)
			}
		}

		// Handle match results.
		if len(matches) == 0 {
			httpErr(w, http.StatusNotFound, "no matching repo found in run")
			return
		}
		if len(matches) > 1 {
			// Multiple repos match the same normalized URL — this is ambiguous.
			httpErr(w, http.StatusConflict, "multiple repos match the given repo_url")
			slog.Warn("pull run repo: multiple matches",
				"run_id", runID,
				"repo_url", req.RepoURL,
				"match_count", len(matches),
			)
			return
		}

		// Single match found — return the pull response.
		match := matches[0]
		resp := pullResponse{
			RunID:         match.RunID,
			RepoID:        match.RepoID,
			RepoTargetRef: match.RepoTargetRef,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("pull run repo: encode response failed", "err", err)
		}

		slog.Info("pull run repo resolved",
			"run_id", runID.String(),
			"repo_id", match.RepoID,
			"repo_url", req.RepoURL,
		)
	}
}

// pullMigRepoHandler resolves a repo_url to execution identifiers for a mig.
// Endpoint: POST /v1/migs/{mig_id}/pull
// Request: {repo_url, mode?}
// Response: 200 OK with {run_id, repo_id, repo_target_ref}
//
// v1 contract:
//   - Server performs the lookup using mig_id + repo_url → mig_repos.id.
//   - Then selects the appropriate run_repos by created_at DESC, filtering by
//     the requested terminal status (Success or Fail).
//   - Mode values:
//   - "last-succeeded" (default): newest run_repos with status=Success
//   - "last-failed": newest run_repos with status=Fail
//   - Uses domaintypes.NormalizeRepoURL for URL comparison.
//   - If no repo matches: 404 error.
//   - If no run with matching status found: 404 error.
func pullMigRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract mig_id from path.
		modID, err := parseParam[domaintypes.MigID](r, "mig_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse request body with strict validation.
		var req modPullRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_url is provided.
		if req.RepoURL == "" {
			httpErr(w, http.StatusBadRequest, "repo_url is required")
			return
		}

		// Normalize the incoming repo_url for comparison.
		normalizedURL := domaintypes.NormalizeRepoURL(req.RepoURL)

		// Determine the target status based on mode.
		// Default is "last-succeeded".
		mode := req.Mode
		if mode == "" {
			mode = "last-succeeded"
		}

		var targetStatus domaintypes.RunRepoStatus
		switch mode {
		case "last-succeeded":
			targetStatus = domaintypes.RunRepoStatusSuccess
		case "last-failed":
			targetStatus = domaintypes.RunRepoStatusFail
		default:
			httpErr(w, http.StatusBadRequest, "invalid mode: %q (must be 'last-succeeded' or 'last-failed')", mode)
			return
		}

		// Verify the mig exists.
		_, err = st.GetMig(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mig not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mig: %v", err)
			slog.Error("pull mig repo: get mig failed", "mig_id", modID, "err", err)
			return
		}

		// List all repos for this mig to find matching repo by normalized URL.
		modRepos, err := st.ListMigReposByMig(r.Context(), modID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list mig repos: %v", err)
			slog.Error("pull mig repo: list mig repos failed", "mig_id", modID, "err", err)
			return
		}

		// Find the repo_id matching the normalized URL.
		var matchedRepoID domaintypes.RepoID
		for _, mr := range modRepos {
			repoURL, err := repoURLForID(r.Context(), st, mr.RepoID)
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("pull mig repo: get repo failed", "mig_id", modID, "repo_id", mr.RepoID, "err", err)
				return
			}
			if domaintypes.NormalizeRepoURL(repoURL) == normalizedURL {
				matchedRepoID = mr.RepoID
				break
			}
		}

		if matchedRepoID.IsZero() {
			httpErr(w, http.StatusNotFound, "no matching repo found in mig")
			return
		}

		// Get the latest run_repos row with the specified terminal status.
		// Select by run_repos.created_at DESC.
		latestRunRepo, err := st.GetLatestRunRepoByMigAndRepoStatus(r.Context(), store.GetLatestRunRepoByMigAndRepoStatusParams{
			MigID:  modID,
			RepoID: matchedRepoID,
			Status: targetStatus,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "no run with status %q found for this repo", mode)
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run repo: %v", err)
			slog.Error("pull mig repo: get latest run repo failed",
				"mig_id", modID,
				"repo_id", matchedRepoID,
				"status", targetStatus,
				"err", err,
			)
			return
		}

		// Return the pull response.
		resp := pullResponse{
			RunID:         latestRunRepo.RunID,
			RepoID:        latestRunRepo.RepoID,
			RepoTargetRef: latestRunRepo.RepoTargetRef,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("pull mig repo: encode response failed", "err", err)
		}

		slog.Info("pull mig repo resolved",
			"mig_id", modID.String(),
			"run_id", latestRunRepo.RunID,
			"repo_id", latestRunRepo.RepoID.String(),
			"mode", mode,
			"repo_url", req.RepoURL,
		)
	}
}
