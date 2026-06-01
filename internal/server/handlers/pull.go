// Package handlers implements HTTP handlers for the ploy server API.
//
// pull.go implements the "pull resolution" endpoints for fetching diffs.
// These endpoints help CLI clients resolve repo execution identifiers needed to
// pull diffs from the server.
//
// Endpoints:
//   - POST /v1/runs/{run_id}/repos/resolve — resolve repo for a specific run
//   - POST /v1/migs/{mig_id}/pull — resolve repo for a mig (last succeeded/failed)
//
// Implements pull resolution endpoints for mig and run repos.
package handlers

import (
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

// runPullRequest is the request body for POST /v1/runs/{run_id}/repos/resolve.
// The client provides a repo_url to resolve to execution identifiers.
type runPullRequest struct {
	RepoURL string `json:"repo_url"`
}

// migPullRequest is the request body for POST /v1/migs/{mig_id}/pull.
// The client provides a repo_url and optional mode to select which run to resolve.
type migPullRequest struct {
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
type pullResponse struct {
	RunID           domaintypes.RunID  `json:"run_id"`
	RepoID          domaintypes.RepoID `json:"repo_id"`
	RepoURL         string             `json:"repo_url,omitempty"`
	SourceCommitSHA string             `json:"source_commit_sha,omitempty"`
}

// -------------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------------

// resolveRunRepoHandler resolves a repo_url to execution identifiers for a specific run.
// Endpoint: POST /v1/runs/{run_id}/repos/resolve
// Request: {repo_url}
// Response: 200 OK with {run_id, repo_id}
//
// v1 contract:
//   - Server matches the repo by joining run_repos to mig_repos by repo_id,
//     filtering by run_id, and comparing normalized repo_url.
//   - Uses domaintypes.NormalizeRepoURL for URL comparison.
//   - If no repo matches: 404 error.
//   - If multiple repos match: 409 error (ambiguous).
func resolveRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}

		var req runPullRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		if req.RepoURL == "" {
			writeHTTPError(w, http.StatusBadRequest, "repo_url is required")
			return
		}

		normalizedURL := domaintypes.NormalizeRepoURL(req.RepoURL)

		run, ok := getRunOrFail(w, r, st, runID, "pull run repo")
		if !ok {
			return
		}

		repoURL, err := repoURLForID(r.Context(), st, run.RepoID)
		if err != nil {
			serverError(w, "pull run", "get repo", err, "run_id", runID, "repo_id", run.RepoID)
			return
		}

		if domaintypes.NormalizeRepoURL(repoURL) != normalizedURL {
			writeHTTPError(w, http.StatusNotFound, "no matching repo found in run")
			return
		}

		resp := pullResponse{
			RunID:           run.ID,
			RepoID:          run.RepoID,
			RepoURL:         repoURL,
			SourceCommitSHA: run.SourceCommitSha,
		}

		writeJSON(w, http.StatusOK, resp)

		slog.Info("pull run repo resolved",
			"run_id", runID.String(),
			"repo_id", run.RepoID,
			"repo_url", req.RepoURL,
		)
	}
}

// pullMigRepoHandler resolves a repo_url to execution identifiers for a mig.
// Endpoint: POST /v1/migs/{mig_id}/pull
// Request: {repo_url, mode?}
// Response: 200 OK with {run_id, repo_id}
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
		migID, ok := parseRequiredPathIDOrWriteError[domaintypes.MigID](w, r, "mig_id")
		if !ok {
			return
		}

		// Parse request body with strict validation.
		var req migPullRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_url is provided.
		if req.RepoURL == "" {
			writeHTTPError(w, http.StatusBadRequest, "repo_url is required")
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

		var targetStatus domaintypes.RunStatus
		switch mode {
		case "last-succeeded":
			targetStatus = domaintypes.RunStatusSuccess
		case "last-failed":
			targetStatus = domaintypes.RunStatusFail
		default:
			writeHTTPError(w, http.StatusBadRequest, "invalid mode: %q (must be 'last-succeeded' or 'last-failed')", mode)
			return
		}

		if _, err := st.GetMig(r.Context(), migID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "mig not found")
				return
			}
			serverError(w, "pull mig repo", "get mig", err, "mig_id", migID)
			return
		}

		migRepos, err := st.ListMigReposByMig(r.Context(), migID)
		if err != nil {
			serverError(w, "pull mig repo", "list mig repos", err, "mig_id", migID)
			return
		}

		// Find the repo_id matching the normalized URL.
		var matchedRepoID domaintypes.RepoID
		for _, mr := range migRepos {
			repoURL, err := repoURLForID(r.Context(), st, mr.RepoID)
			if err != nil {
				serverError(w, "pull mig repo", "get repo", err, "mig_id", migID, "repo_id", mr.RepoID)
				return
			}
			if domaintypes.NormalizeRepoURL(repoURL) == normalizedURL {
				matchedRepoID = mr.RepoID
				break
			}
		}

		if matchedRepoID.IsZero() {
			writeHTTPError(w, http.StatusNotFound, "no matching repo found in mig")
			return
		}

		// Get the latest run_repos row with the specified terminal status.
		// Select by run_repos.created_at DESC.
		latestRunRepo, err := st.GetLatestRunByMigAndRepoStatus(r.Context(), store.GetLatestRunByMigAndRepoStatusParams{
			MigID:  migID,
			RepoID: matchedRepoID,
			Status: targetStatus,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "no run with status %q found for this repo", mode)
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run repo: %v", err)
			slog.Error("pull mig repo: get latest run repo failed",
				"mig_id", migID,
				"repo_id", matchedRepoID,
				"status", targetStatus,
				"err", err,
			)
			return
		}

		// Return the pull response.
		resp := pullResponse{
			RunID:  latestRunRepo.RunID,
			RepoID: latestRunRepo.RepoID,
		}

		writeJSON(w, http.StatusOK, resp)

		slog.Info("pull mig repo resolved",
			"mig_id", migID.String(),
			"run_id", latestRunRepo.RunID,
			"repo_id", latestRunRepo.RepoID.String(),
			"mode", mode,
			"repo_url", req.RepoURL,
		)
	}
}
