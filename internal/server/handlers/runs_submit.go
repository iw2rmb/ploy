package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// createSingleRepoRunHandler submits a single-repo run and queues it for scheduler-driven execution.
// Endpoint: POST /v1/runs
// Request: {repo_url, base_ref, target_ref, spec}
// Response: 201 Created with {run_id, mig_id, spec_id}
//
// v1 contract:
// - Submits a single-repo run via POST /v1/runs.
// - Creates a mig project as a side-effect; mig name == mig id.
// - Creates an initial spec row and sets migs.spec_id.
// - Creates a mig repo row for the provided repo_url.
// - Creates a run and queued run_repo row.
// - Job materialization is deferred to the batch scheduler/start endpoint and gated on prep readiness.
//
// This handler replaces the previous POST /v1/migs endpoint for run submission.
func createSingleRepoRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	// Spec can be large (JSON blobs), so we allow up to 4 MiB.
	const maxBodySize = 4 << 20
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with strict validation and domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req domainapi.RunSubmitRequest
		if err := decodeRequestJSON(w, r, &req, maxBodySize); err != nil {
			return
		}
		var createdByPtr *string
		if req.CreatedBy != "" {
			createdByPtr = &req.CreatedBy
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		if !validateField(w, "repo_url", req.RepoURL) ||
			!validateField(w, "base_ref", req.BaseRef) ||
			!validateField(w, "target_ref", req.TargetRef) {
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
		sourceCommitSHA, seedErr := resolveSourceCommitSHAFromContext(r.Context(), rawRepoURL, req.BaseRef.String(), gitAuthOptionsFromSpec(req.Spec))
		if seedErr != nil {
			writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", normalizedRepoURL, req.BaseRef.String(), seedErr)
			slog.Error("create single-repo run: resolve source commit failed",
				"repo_url", normalizedRepoURL,
				"base_ref", req.BaseRef.String(),
				"err", seedErr,
			)
			return
		}

		// v1 side-effect: Create spec row
		specID := domaintypes.NewSpecID()
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

		// v1 side-effect: Create mig project with name == id
		migID := domaintypes.NewMigID()
		if _, err := st.CreateMig(r.Context(), store.CreateMigParams{
			ID:        migID,
			Name:      migID.String(),
			SpecID:    &createdSpec.ID,
			CreatedBy: createdByPtr,
		}); err != nil {
			serverError(w, "create single-repo run", "create mig", err, "mig_id", migID)
			return
		}

		// Create mig repo for the provided repo_url.
		// Persist normalized URL without embedded credentials.
		migRepoID := domaintypes.NewMigRepoID()
		migRepo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:        migRepoID,
			MigID:     migID,
			Url:       normalizedRepoURL,
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			serverError(w, "create single-repo run", "create mig repo", err, "mig_id", migID, "repo_url", normalizedRepoURL)
			return
		}

		// Create run
		runID := domaintypes.NewRunID()
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        runID,
			MigID:     migID,
			SpecID:    createdSpec.ID,
			CreatedBy: createdByPtr,
		})
		if err != nil {
			serverError(w, "create single-repo run", "create run", err, "run_id", runID)
			return
		}

		// Create run_repo entry.
		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			MigID:           migID,
			RunID:           run.ID,
			RepoID:          migRepo.RepoID,
			RepoBaseRef:     migRepo.BaseRef,
			RepoTargetRef:   migRepo.TargetRef,
			SourceCommitSha: sourceCommitSHA,
			RepoSha0:        sourceCommitSHA,
		})
		if err != nil {
			serverError(w, "create single-repo run", "create run repo", err, "run_id", run.ID, "repo_id", migRepo.RepoID)
			return
		}

		// Build response with run_id, mig_id, and spec_id.
		resp := struct {
			RunID  domaintypes.RunID  `json:"run_id"`
			MigID  domaintypes.MigID  `json:"mig_id"`
			SpecID domaintypes.SpecID `json:"spec_id"`
		}{
			RunID:  run.ID,
			MigID:  migID,
			SpecID: createdSpec.ID,
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
					"repo_base_ref":   migRepo.BaseRef,
					"repo_target_ref": migRepo.TargetRef,
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
			"mig_id", migID.String(),
			"spec_id", createdSpec.ID,
			"repo_id", runRepo.RepoID,
			"repo_url", normalizedRepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
		)
	}
}
