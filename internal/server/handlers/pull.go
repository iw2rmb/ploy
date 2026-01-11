// Package handlers implements HTTP handlers for the ploy server API.
//
// pull.go implements the "pull resolution" endpoints for fetching diffs.
// These endpoints help CLI clients resolve repo execution identifiers needed to
// pull diffs from the server.
//
// Endpoints:
//   - POST /v1/runs/{run_id}/pull — resolve repo for a specific run
//   - POST /v1/mods/{mod_id}/pull — resolve repo for a mod (last succeeded/failed)
//
// Implements pull resolution endpoints for mod and run repos.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/vcs"
)

// -------------------------------------------------------------------------
// Request/Response types for pull resolution endpoints
// -------------------------------------------------------------------------

// runPullRequest is the request body for POST /v1/runs/{run_id}/pull.
// The client provides a repo_url to resolve to execution identifiers.
type runPullRequest struct {
	RepoURL string `json:"repo_url"`
}

// modPullRequest is the request body for POST /v1/mods/{mod_id}/pull.
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
//   - repo_id: the mod_repos.id for the matched repo
//   - repo_target_ref: the target ref snapshot from run_repos
type pullResponse struct {
	RunID         domaintypes.RunID     `json:"run_id"`
	RepoID        domaintypes.ModRepoID `json:"repo_id"`
	RepoTargetRef string                `json:"repo_target_ref"`
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
//   - Server matches the repo by joining run_repos to mod_repos by repo_id,
//     filtering by run_id, and comparing normalized repo_url.
//   - Uses vcs.NormalizeRepoURL for URL comparison.
//   - If no repo matches: 404 error.
//   - If multiple repos match: 409 error (ambiguous).
func pullRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run_id from path.
		runID, err := domaintypes.ParseRunIDParam(r, "run_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Parse request body with strict validation.
		var req runPullRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_url is provided.
		if req.RepoURL == "" {
			http.Error(w, "repo_url is required", http.StatusBadRequest)
			return
		}

		// Normalize the incoming repo_url for comparison.
		normalizedURL := vcs.NormalizeRepoURL(req.RepoURL)

		// Verify the run exists before querying repos.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("pull run repo: get run failed", "run_id", runID, "err", err)
			return
		}

		// List all repos in this run with their URLs.
		// We need to iterate and compare normalized URLs to find matches.
		runRepos, err := st.ListRunReposWithURLByRun(r.Context(), runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("pull run repo: list run repos failed", "run_id", runID, "err", err)
			return
		}

		// Find matching repos by normalized URL.
		var matches []store.ListRunReposWithURLByRunRow
		for _, rr := range runRepos {
			if vcs.NormalizeRepoURL(rr.RepoUrl) == normalizedURL {
				matches = append(matches, rr)
			}
		}

		// Handle match results.
		if len(matches) == 0 {
			http.Error(w, "no matching repo found in run", http.StatusNotFound)
			return
		}
		if len(matches) > 1 {
			// Multiple repos match the same normalized URL — this is ambiguous.
			http.Error(w, "multiple repos match the given repo_url", http.StatusConflict)
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

// pullModRepoHandler resolves a repo_url to execution identifiers for a mod.
// Endpoint: POST /v1/mods/{mod_id}/pull
// Request: {repo_url, mode?}
// Response: 200 OK with {run_id, repo_id, repo_target_ref}
//
// v1 contract:
//   - Server performs the lookup using mod_id + repo_url → mod_repos.id.
//   - Then selects the appropriate run_repos by created_at DESC, filtering by
//     the requested terminal status (Success or Fail).
//   - Mode values:
//   - "last-succeeded" (default): newest run_repos with status=Success
//   - "last-failed": newest run_repos with status=Fail
//   - Uses vcs.NormalizeRepoURL for URL comparison.
//   - If no repo matches: 404 error.
//   - If no run with matching status found: 404 error.
func pullModRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract mod_id from path.
		modID, err := domaintypes.ParseModIDParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Parse request body with strict validation.
		var req modPullRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_url is provided.
		if req.RepoURL == "" {
			http.Error(w, "repo_url is required", http.StatusBadRequest)
			return
		}

		// Normalize the incoming repo_url for comparison.
		normalizedURL := vcs.NormalizeRepoURL(req.RepoURL)

		// Determine the target status based on mode.
		// Default is "last-succeeded".
		mode := req.Mode
		if mode == "" {
			mode = "last-succeeded"
		}

		var targetStatus store.RunRepoStatus
		switch mode {
		case "last-succeeded":
			targetStatus = store.RunRepoStatusSuccess
		case "last-failed":
			targetStatus = store.RunRepoStatusFail
		default:
			http.Error(w, fmt.Sprintf("invalid mode: %q (must be 'last-succeeded' or 'last-failed')", mode), http.StatusBadRequest)
			return
		}

		// Verify the mod exists.
		_, err = st.GetMod(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("pull mod repo: get mod failed", "mod_id", modID, "err", err)
			return
		}

		// List all repos for this mod to find matching repo by normalized URL.
		modRepos, err := st.ListModReposByMod(r.Context(), modID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list mod repos: %v", err), http.StatusInternalServerError)
			slog.Error("pull mod repo: list mod repos failed", "mod_id", modID, "err", err)
			return
		}

		// Find the repo_id matching the normalized URL.
		var matchedRepoID domaintypes.ModRepoID
		for _, mr := range modRepos {
			if vcs.NormalizeRepoURL(mr.RepoUrl) == normalizedURL {
				matchedRepoID = mr.ID
				break
			}
		}

		if matchedRepoID.IsZero() {
			http.Error(w, "no matching repo found in mod", http.StatusNotFound)
			return
		}

		// Get the latest run_repos row with the specified terminal status.
		// Select by run_repos.created_at DESC.
		latestRunRepo, err := st.GetLatestRunRepoByModAndRepoStatus(r.Context(), store.GetLatestRunRepoByModAndRepoStatusParams{
			ModID:  modID,
			RepoID: matchedRepoID,
			Status: targetStatus,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, fmt.Sprintf("no run with status %q found for this repo", mode), http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run repo: %v", err), http.StatusInternalServerError)
			slog.Error("pull mod repo: get latest run repo failed",
				"mod_id", modID,
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
			slog.Error("pull mod repo: encode response failed", "err", err)
		}

		slog.Info("pull mod repo resolved",
			"mod_id", modID.String(),
			"run_id", latestRunRepo.RunID,
			"repo_id", latestRunRepo.RepoID.String(),
			"mode", mode,
			"repo_url", req.RepoURL,
		)
	}
}
