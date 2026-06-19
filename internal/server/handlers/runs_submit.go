package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5"
)

var submitCommitSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// createSingleRepoRunHandler submits a single-repo run and queues it for scheduler-driven execution.
// Endpoint: POST /v1/runs
// Request: {repo_url, ref, commit_sha?, spec}
// Response: 201 Created with {wave_id, run_id, mig_id, spec_id}
//
// v1 contract:
// - Submits a single-repo run via POST /v1/runs.
// - Creates a mig project as a side-effect; mig name == mig id.
// - Creates an initial spec row and sets migs.spec_id.
// - Creates a mig repo row for the provided repo_url.
// - Creates a wave and one queued run row.
// - Job materialization is deferred to the wave scheduler and gated on prep readiness.
//
// This handler replaces the previous POST /v1/migs endpoint for run submission.
func createSingleRepoRunHandler(st store.Store, eventsService *events.Service, gitAuth gitauth.Options) http.HandlerFunc {
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
		if commitSHA != "" && !submitCommitSHARe.MatchString(commitSHA) {
			writeHTTPError(w, http.StatusBadRequest, "commit_sha must be a lowercase 40-hex sha")
			return
		}

		// Validate spec (cannot be empty for single-repo run submission)
		if len(req.Spec) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "spec is required")
			return
		}
		if _, err := contracts.ParseMigSpecJSON(req.Spec); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "spec: %v", err)
			return
		}

		// Resolve the source SHA before creating durable rows. A failed remote
		// query must reject the submit instead of leaving a run with no repos.
		rawRepoURL := strings.TrimSpace(req.RepoURL.String())
		normalizedRepoURL := domaintypes.NormalizeRepoURL(rawRepoURL)
		sourceCommitSHA := commitSHA
		if sourceCommitSHA == "" {
			var seedErr error
			sourceCommitSHA, seedErr = resolveSourceCommitSHAFromContext(r.Context(), rawRepoURL, sourceRef, gitAuth)
			if seedErr != nil {
				writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", normalizedRepoURL, sourceRef, seedErr)
				slog.Error("create single-repo run: resolve source commit failed",
					"repo_url", normalizedRepoURL,
					"ref", sourceRef,
					"err", seedErr,
				)
				return
			}
		}

		specID := req.SpecID
		if specID.IsZero() {
			specID = domaintypes.NewSpecID()
			createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
				ID:        specID,
				Name:      "",
				Spec:      req.Spec,
				CreatedBy: createdByPtr,
			})
			if err != nil {
				serverError(w, "create single-repo run", "create spec", err)
				return
			}
			specID = createdSpec.ID
		} else if _, err := st.GetSpec(r.Context(), specID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusBadRequest, "spec_id not found")
				return
			}
			serverError(w, "create single-repo run", "get spec", err, "spec_id", specID)
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
		wave, runs, err := st.CreateWaveWithRuns(r.Context(), store.CreateWaveWithRunsParams{
			Wave: store.CreateWaveParams{
				ID:        waveID,
				MigID:     migID,
				SpecID:    specID,
				CreatedBy: createdByPtr,
			},
			Runs: []store.CreateRunParams{{
				ID:              runID,
				WaveID:          waveID,
				MigID:           migID,
				SpecID:          specID,
				RepoID:          migRepo.RepoID,
				RepoBaseRef:     migRepo.BaseRef,
				SourceCommitSha: sourceCommitSHA,
				RepoSha0:        sourceCommitSHA,
				CreatedBy:       createdByPtr,
			}},
		})
		if err != nil {
			serverError(w, "create single-repo run", "create run", err, "run_id", runID)
			return
		}
		run := runs[0]

		resp := domainapi.CreateSingleRepoRunResponse{
			WaveID: wave.ID,
			RunID:  run.ID,
			MigID:  migID,
			SpecID: specID,
		}

		// Publish queued event to SSE hub for the run
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
				Stages:    make(map[domaintypes.JobID]migsapi.StageStatus),
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
