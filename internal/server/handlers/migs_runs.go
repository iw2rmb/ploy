package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// createMigRunHandler creates a batch run from the mig + spec + selected repos.
// Endpoint: POST /v1/migs/{mig_id}/runs
// Request: {repo_selector: {mode, repos?}, created_by?}
// Response: 201 Created with {run_id}
//
// v1 contract:
// - repo_selector.mode: "all" | "failed" | "explicit"
// - For "failed": selects repos whose last terminal run_repos status is 'Fail'.
// - For "explicit": uses repo_selector.repos array of repo_urls.
// - Must use migs.spec_id; if NULL, return error.
// - Archived migs cannot be executed.
// - Copies migs.spec_id → runs.spec_id for immutability.
// - Creates run_repos rows snapshotting refs from mig_repos.
// - Job materialization is deferred to the batch scheduler/start endpoint and gated on prep readiness.
func createMigRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse mig_id from URL path.
		modID, err := parseParam[domaintypes.MigID](r, "mig_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse request body with strict validation.
		var req struct {
			RepoSelector struct {
				Mode  string   `json:"mode"`            // "all" | "failed" | "explicit"
				Repos []string `json:"repos,omitempty"` // repo_urls for "explicit" mode
			} `json:"repo_selector"`
			CreatedBy *string `json:"created_by,omitempty"`
		}
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_selector.mode is one of the allowed values.
		switch req.RepoSelector.Mode {
		case "all", "failed", "explicit":
			// Valid modes.
		default:
			httpErr(w, http.StatusBadRequest, `repo_selector.mode must be "all", "failed", or "explicit"`)
			return
		}

		// For explicit mode, validate repos array is non-empty.
		if req.RepoSelector.Mode == "explicit" && len(req.RepoSelector.Repos) == 0 {
			httpErr(w, http.StatusBadRequest, "repo_selector.repos must be non-empty for explicit mode")
			return
		}

		// Verify mig exists and is not archived.
		mig, err := st.GetMig(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mig not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mig: %v", err)
			slog.Error("create mig run: get mig failed", "mig_id", modID.String(), "err", err)
			return
		}

		// Archived migs cannot be executed.
		if mig.ArchivedAt.Valid {
			httpErr(w, http.StatusConflict, "cannot create run for archived mig")
			return
		}

		// Validate migs.spec_id is non-NULL.
		if mig.SpecID == nil {
			httpErr(w, http.StatusBadRequest, "mig has no spec; set a spec before creating runs")
			return
		}

		// Select repos based on mode.
		selectedRepos, err := selectReposForRun(r.Context(), st, modID, req.RepoSelector.Mode, req.RepoSelector.Repos)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to select repos: %v", err)
			slog.Error("create mig run: select repos failed", "mig_id", modID.String(), "mode", req.RepoSelector.Mode, "err", err)
			return
		}

		// If no repos are selected, return an error.
		if len(selectedRepos) == 0 {
			httpErr(w, http.StatusBadRequest, "no repos selected for run")
			return
		}

		// Create run with spec_id copied from migs.spec_id for immutability.
		runID := domaintypes.NewRunID()
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        runID,
			MigID:     modID,
			SpecID:    *mig.SpecID,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create run: %v", err)
			slog.Error("create mig run: create run failed", "mig_id", modID.String(), "run_id", runID, "err", err)
			return
		}

		// Create run_repos entries for each selected repo.
		// v1: run_repos snapshots refs from mig_repos at run creation time.
		for _, modRepo := range selectedRepos {
			repoURL, urlErr := repoURLForID(r.Context(), st, modRepo.RepoID)
			if urlErr != nil {
				httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", urlErr)
				slog.Error("create mig run: get repo failed", "repo_id", modRepo.RepoID, "err", urlErr)
				return
			}
			sourceCommitSHA, seedErr := resolveSourceCommitSHAFromContext(r.Context(), repoURL, modRepo.BaseRef)
			if seedErr != nil {
				httpErr(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", repoURL, modRepo.BaseRef, seedErr)
				slog.Error("create mig run: resolve source commit failed",
					"run_id", run.ID,
					"repo_id", modRepo.RepoID,
					"repo_url", repoURL,
					"base_ref", modRepo.BaseRef,
					"err", seedErr,
				)
				return
			}
			// Create run_repo entry snapshotting refs.
			_, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
				MigID:           modID,
				RunID:           run.ID,
				RepoID:          modRepo.RepoID,
				RepoBaseRef:     modRepo.BaseRef,
				RepoTargetRef:   modRepo.TargetRef,
				SourceCommitSha: sourceCommitSHA,
				RepoSha0:        sourceCommitSHA,
			})
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to create run repo: %v", err)
				slog.Error("create mig run: create run repo failed",
					"run_id", run.ID,
					"repo_id", modRepo.RepoID,
					"repo_url", repoURL,
					"err", err,
				)
				return
			}

		}

		// Build response with run_id.
		resp := struct {
			RunID domaintypes.RunID `json:"run_id"`
		}{
			RunID: run.ID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mig run: encode response failed", "err", err)
		}

		slog.Info("mig run created",
			"run_id", run.ID,
			"mig_id", modID.String(),
			"spec_id", *mig.SpecID,
			"repo_count", len(selectedRepos),
			"mode", req.RepoSelector.Mode,
		)
	}
}

// selectReposForRun selects mig repos based on the selection mode.
// Returns a list of mig_repos to include in the run.
//
// Modes:
// - "all": all repos in the mig's repo set
// - "failed": repos whose last terminal run_repos status is 'Fail'
// - "explicit": specific repos by URL (normalized for matching)
func selectReposForRun(ctx context.Context, st store.Store, modID domaintypes.MigID, mode string, repoURLs []string) ([]store.MigRepo, error) {
	// Get all repos for the mig.
	allRepos, err := st.ListMigReposByMig(ctx, modID)
	if err != nil {
		return nil, fmt.Errorf("list mig repos: %w", err)
	}

	switch mode {
	case "all":
		// Return all repos in the mig's repo set.
		return allRepos, nil

	case "failed":
		// Get repo IDs whose last terminal status is 'Fail'.
		failedRepoIDs, err := st.ListFailedRepoIDsByMig(ctx, modID)
		if err != nil {
			return nil, fmt.Errorf("list failed repos: %w", err)
		}

		// Build a set of failed repo IDs for efficient lookup.
		failedSet := make(map[domaintypes.RepoID]bool, len(failedRepoIDs))
		for _, repoID := range failedRepoIDs {
			failedSet[repoID] = true
		}

		// Filter allRepos to only include failed ones.
		var failedRepos []store.MigRepo
		for _, repo := range allRepos {
			if failedSet[repo.RepoID] {
				failedRepos = append(failedRepos, repo)
			}
		}
		return failedRepos, nil

	case "explicit":
		// Build a set of normalized URLs for matching.
		// Use domaintypes.NormalizeRepoURL for URL comparison.
		normalizedURLs := make(map[string]bool, len(repoURLs))
		for _, url := range repoURLs {
			normalizedURLs[domaintypes.NormalizeRepoURL(url)] = true
		}

		// Filter allRepos to only include those with matching URLs.
		var explicitRepos []store.MigRepo
		for _, repo := range allRepos {
			repoURL, err := repoURLForID(ctx, st, repo.RepoID)
			if err != nil {
				return nil, fmt.Errorf("get repo %s: %w", repo.RepoID, err)
			}
			if normalizedURLs[domaintypes.NormalizeRepoURL(repoURL)] {
				explicitRepos = append(explicitRepos, repo)
			}
		}
		return explicitRepos, nil

	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}
}
