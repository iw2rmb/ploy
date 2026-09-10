package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
)

// createSingleRepoRunHandler submits a single-repo run with its complete job chain.
// Endpoint: POST /v1/runs
// Request: {repo_url, ref, commit_sha?, spec|spec_selector, spec_overrides?}
// Response: 201 Created with {wave_id, run_id, mig_id, spec_id}
//
// v1 contract:
// - Submits a single-repo run via POST /v1/runs.
// - Creates a mig project as a side-effect; mig name == mig id.
// - Creates an initial spec row and sets migs.spec_id.
// - Creates a mig repo row for the provided repo_url.
// - Atomically creates a wave, one running run row, and its job chain.
//
// This handler replaces the previous POST /v1/migs endpoint for run submission.
func createSingleRepoRunHandler(st store.Store, eventsService *events.Service, gitAuth gitauth.Options, specServices runSubmitSpecServices, registries ...*gitlabtokens.Registry) http.HandlerFunc {
	tokenRegistry := optionalGitLabTokenRegistry(registries)
	// Spec can be large (JSON blobs), so we allow up to 4 MiB.
	const maxBodySize = 4 << 20
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with strict validation and domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req domainapi.RunSubmitRequest
		if err := decodeRequestJSON(w, r, &req, maxBodySize); err != nil {
			return
		}
		createdByPtr, err := resolvedCreatedBy(r.Context(), st, req.CreatedBy)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to resolve caller identity: %v", err)
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		if !validateField(w, "repo_url", req.RepoURL) ||
			!validateField(w, "ref", req.Ref) {
			return
		}
		sourceRef := req.Ref.String()
		commitSHA := strings.TrimSpace(req.CommitSHA)
		if commitSHA != "" && !domaintypes.IsCanonicalFullCommitSHA(req.CommitSHA) {
			writeHTTPError(w, http.StatusBadRequest, "commit_sha must be a lowercase 40-hex sha")
			return
		}
		if err := validateRunSubmissionSpecRequest(req); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}

		// Resolve the source SHA before creating durable rows. A failed remote
		// query must reject the submit instead of leaving a run with no repos.
		rawRepoURL := strings.TrimSpace(req.RepoURL.String())
		normalizedRepoURL := domaintypes.NormalizeRepoURL(rawRepoURL)
		gitLabTokenHash, gitLabToken, err := validateGitLabTokenRequestForRepos(req.GitLabToken, gitAuth.GitLabDomain, []string{rawRepoURL})
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}
		sourceCommitSHA := commitSHA
		if sourceCommitSHA == "" {
			var seedErr error
			resolveAuth := gitAuthWithEphemeralToken(gitAuth, gitLabToken)
			sourceCommitSHA, seedErr = resolveSourceCommitSHAFromContext(r.Context(), rawRepoURL, sourceRef, resolveAuth)
			if seedErr != nil {
				if gitLabToken != "" {
					writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s using provided GitLab token: %v", normalizedRepoURL, sourceRef, seedErr)
				} else {
					writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", normalizedRepoURL, sourceRef, seedErr)
				}
				slog.Error("create single-repo run: resolve source commit failed",
					"repo_url", normalizedRepoURL,
					"ref", sourceRef,
					"err", seedErr,
				)
				return
			}
		}

		submissionSpec, err := resolveRunSubmissionSpec(r.Context(), req, specServices)
		if err != nil {
			writeRunSubmissionSpecError(w, err)
			return
		}
		plannedJobs, err := planJobsFromSpec(submissionSpec.canonical)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "invalid run job plan: %v", err)
			return
		}
		specID, err := persistRunSubmissionSpec(r.Context(), st, submissionSpec, createdByPtr)
		if err != nil {
			serverError(w, "create single-repo run", "persist spec snapshot", err)
			return
		}

		// v1 side-effect: Create mig project with name == id
		migID := domaintypes.NewMigID()
		if _, err := st.CreateMig(r.Context(), store.CreateMigParams{
			ID:        migID,
			Name:      migID.String(),
			SpecID:    &specID,
			CreatedBy: createdByPtr,
		}); err != nil {
			serverError(w, "create single-repo run", "create mig", err, "mig_id", migID)
			return
		}

		// Create mig repo for the provided repo_url.
		// Persist normalized URL without embedded credentials.
		migRepoID := domaintypes.NewMigRepoID()
		migRepo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:      migRepoID,
			MigID:   migID,
			Url:     normalizedRepoURL,
			BaseRef: sourceRef,
		})
		if err != nil {
			serverError(w, "create single-repo run", "create mig repo", err, "mig_id", migID, "repo_url", normalizedRepoURL)
			return
		}

		runID := domaintypes.NewRunID()
		waveID := domaintypes.WaveID(runID.String())
		runStats, err := gitlabtoken.RunStatsWithMarker(gitLabTokenHash)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}
		if gitLabTokenHash != "" {
			tokenRegistry.Register(gitLabTokenHash, gitLabToken, []domaintypes.RunID{runID})
		}
		wave, runs, err := st.CreateWaveWithRuns(r.Context(), store.CreateWaveWithRunsParams{
			Wave: store.CreateWaveParams{
				ID:        waveID,
				MigID:     migID,
				SpecID:    specID,
				CreatedBy: createdByPtr,
			},
			Runs: []store.RunPlan{{
				ID:              runID,
				RepoID:          migRepo.RepoID,
				RepoBaseRef:     migRepo.BaseRef,
				SourceCommitSha: sourceCommitSHA,
				RepoSha0:        sourceCommitSHA,
				Stats:           runStats,
				Jobs:            plannedJobs,
			}},
		})
		if err != nil {
			if gitLabTokenHash != "" {
				tokenRegistry.ReleaseRuns([]domaintypes.RunID{runID})
			}
			serverError(w, "create single-repo run", "create run", err, "run_id", runID)
			return
		}
		materialized := runs[0]
		run := materialized.Run

		resp := domainapi.CreateSingleRepoRunResponse{
			WaveID: wave.ID,
			RunID:  run.ID,
			MigID:  migID,
			SpecID: specID,
		}

		// Publish the materialized run to the SSE hub.
		if eventsService != nil {
			// Build a minimal run summary for the event using migsapi.RunSummary
			// (matching the event structure expected by eventsService.PublishRun)
			runState, convErr := migsapi.RunStatusFromDomain(run.Status)
			if convErr != nil {
				slog.Error("create single-repo run: invalid run status for publish payload", "run_id", run.ID, "status", run.Status, "err", convErr)
				runState = migsapi.RunStateRunning
			}
			summary := migsapi.RunSummary{
				RunID:      run.ID,
				State:      runState,
				Submitter:  "",
				Repository: normalizedRepoURL,
				Metadata: map[string]string{
					"repo_id":           run.RepoID.String(),
					"repo_base_ref":     migRepo.BaseRef,
					"source_commit_sha": run.SourceCommitSha,
				},
				CreatedAt: timeOrZero(run.CreatedAt),
				UpdatedAt: time.Now().UTC(),
				Stages:    make(map[domaintypes.JobID]migsapi.StageStatus, len(materialized.Jobs)),
			}
			for _, job := range materialized.Jobs {
				stageState, convErr := migsapi.StageStatusFromDomain(job.Status)
				if convErr != nil {
					slog.Error("create single-repo run: invalid job status for publish payload", "run_id", run.ID, "job_id", job.ID, "status", job.Status, "err", convErr)
					continue
				}
				summary.Stages[job.ID] = migsapi.StageStatus{
					State:       stageState,
					Attempts:    1,
					MaxAttempts: 1,
					Artifacts:   map[string]string{},
					NextID:      job.NextID,
				}
			}
			if err := eventsService.PublishRun(r.Context(), run.ID, summary); err != nil {
				slog.Error("create single-repo run: publish run event failed", "run_id", run.ID, "err", err)
			}
		}

		writeJSON(w, http.StatusCreated, resp)

		slog.Info("single-repo run created",
			"run_id", run.ID,
			"wave_id", wave.ID,
			"mig_id", migID.String(),
			"spec_id", specID,
			"repo_id", run.RepoID,
			"repo_url", normalizedRepoURL,
			"ref", sourceRef,
		)
	}
}
