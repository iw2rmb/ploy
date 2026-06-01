package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/store"
)

// createMigRunHandler creates a launch wave from the mig + spec + selected repos.
// Endpoint: POST /v1/migs/{mig_id}/waves
// Request: {repo_selector: {mode, repos?}, created_by?}
// Response: 201 Created with {wave_id}
//
// v1 contract:
// - repo_selector.mode: "all" | "failed" | "explicit"
// - For "failed": selects repos whose last terminal run status is 'Fail'.
// - For "explicit": uses repo_selector.repos array of repo_urls.
// - Must use migs.spec_id; if NULL, return error.
// - Archived migs cannot be executed.
// - Copies migs.spec_id → runs.spec_id for immutability.
// - Creates run rows snapshotting source refs from mig_repos.
// - Job materialization is deferred to the batch scheduler and gated on prep readiness.
func createMigRunHandler(st store.Store, gitAuth gitauth.Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request body with strict validation.
		var req struct {
			RepoSelector struct {
				Mode  string   `json:"mode"`            // "all" | "failed" | "explicit"
				Repos []string `json:"repos,omitempty"` // repo_urls for "explicit" mode
			} `json:"repo_selector"`
			CreatedBy *string `json:"created_by,omitempty"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate repo_selector.mode is one of the allowed values.
		switch req.RepoSelector.Mode {
		case "all", "failed", "explicit":
			// Valid modes.
		default:
			writeHTTPError(w, http.StatusBadRequest, `repo_selector.mode must be "all", "failed", or "explicit"`)
			return
		}

		// For explicit mode, validate repos array is non-empty.
		if req.RepoSelector.Mode == "explicit" && len(req.RepoSelector.Repos) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "repo_selector.repos must be non-empty for explicit mode")
			return
		}

		// Verify mig exists and is not archived.
		mig, ok := getMigByIDOrFail(w, r, st, "create mig run")
		if !ok {
			return
		}
		migID := mig.ID
		if mig.ArchivedAt.Valid {
			writeHTTPError(w, http.StatusConflict, "cannot create run for archived mig")
			return
		}

		// Validate migs.spec_id is non-NULL.
		if mig.SpecID == nil {
			writeHTTPError(w, http.StatusBadRequest, "mig has no spec; set a spec before creating runs")
			return
		}

		// Select repos based on mode.
		selectedRepos, err := selectReposForRun(r.Context(), st, migID, req.RepoSelector.Mode, req.RepoSelector.Repos)
		if err != nil {
			serverError(w, "create mig run", "select repos", err, "mig_id", migID.String(), "mode", req.RepoSelector.Mode)
			return
		}

		// If no repos are selected, return an error.
		if len(selectedRepos) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "no repos selected for run")
			return
		}

		waveID := domaintypes.NewWaveID()
		runs := make([]store.CreateRunParams, 0, len(selectedRepos))
		for _, migRepo := range selectedRepos {
			runID := domaintypes.NewRunID()
			repoURL, urlErr := repoURLForID(r.Context(), st, migRepo.RepoID)
			if urlErr != nil {
				serverError(w, "create mig run", "get repo", urlErr, "repo_id", migRepo.RepoID)
				return
			}
			sourceCommitSHA, seedErr := resolveSourceCommitSHAFromContext(r.Context(), repoURL, migRepo.BaseRef, gitAuth)
			if seedErr != nil {
				writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", repoURL, migRepo.BaseRef, seedErr)
				slog.Error("create mig run: resolve source commit failed",
					"run_id", runID,
					"repo_id", migRepo.RepoID,
					"repo_url", repoURL,
					"base_ref", migRepo.BaseRef,
					"err", seedErr,
				)
				return
			}
			runs = append(runs, store.CreateRunParams{
				ID:              runID,
				WaveID:          waveID,
				MigID:           migID,
				SpecID:          *mig.SpecID,
				RepoID:          migRepo.RepoID,
				RepoBaseRef:     migRepo.BaseRef,
				SourceCommitSha: sourceCommitSHA,
				RepoSha0:        sourceCommitSHA,
			})
		}

		wave, _, err := st.CreateWaveWithRuns(r.Context(), store.CreateWaveWithRunsParams{
			Wave: store.CreateWaveParams{
				ID:        waveID,
				MigID:     migID,
				SpecID:    *mig.SpecID,
				CreatedBy: req.CreatedBy,
			},
			Runs: runs,
		})
		if err != nil {
			serverError(w, "create mig wave", "create wave with runs", err, "mig_id", migID.String(), "wave_id", waveID)
			return
		}

		// Build response with wave_id.
		resp := struct {
			WaveID domaintypes.WaveID `json:"wave_id"`
		}{
			WaveID: wave.ID,
		}

		writeJSON(w, http.StatusCreated, resp)

		slog.Info("mig run created",
			"wave_id", wave.ID,
			"mig_id", migID.String(),
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
// - "failed": repos whose last terminal run status is 'Fail'
// - "explicit": specific repos by URL (normalized for matching)
func selectReposForRun(ctx context.Context, st store.Store, migID domaintypes.MigID, mode string, repoURLs []string) ([]store.MigRepo, error) {
	// Get all repos for the mig.
	allRepos, err := st.ListMigReposByMig(ctx, migID)
	if err != nil {
		return nil, fmt.Errorf("list mig repos: %w", err)
	}

	switch mode {
	case "all":
		// Return all repos in the mig's repo set.
		return allRepos, nil

	case "failed":
		// Get repo IDs whose last terminal status is 'Fail'.
		failedRepoIDs, err := st.ListFailedRepoIDsByMig(ctx, migID)
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
