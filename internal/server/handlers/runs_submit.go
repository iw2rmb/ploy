package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// createSingleRepoRunHandler submits a single-repo run and immediately starts execution.
// Endpoint: POST /v1/runs
// Request: {repo_url, base_ref, target_ref, spec}
// Response: 201 Created with {run_id, mig_id, spec_id}
//
// v1 contract:
// - Submits a single-repo run via POST /v1/runs.
// - Creates a mig project as a side-effect; mig name == mig id.
// - Creates an initial spec row and sets migs.spec_id.
// - Creates a mig repo row for the provided repo_url.
// - Creates a run and starts execution immediately.
//
// This handler replaces the previous POST /v1/migs endpoint for run submission.
func createSingleRepoRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	// Spec can be large (JSON blobs), so we allow up to 4 MiB.
	const maxBodySize = 4 << 20
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with strict validation and domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req struct {
			RepoURL   domaintypes.RepoURL `json:"repo_url"`
			BaseRef   domaintypes.GitRef  `json:"base_ref"`
			TargetRef domaintypes.GitRef  `json:"target_ref"`
			Spec      json.RawMessage     `json:"spec"`
			CreatedBy *string             `json:"created_by,omitempty"`
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		if err := req.RepoURL.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "repo_url: %v", err)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "base_ref: %v", err)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "target_ref: %v", err)
			return
		}

		// Validate spec (cannot be empty for single-repo run submission)
		if len(req.Spec) == 0 {
			httpErr(w, http.StatusBadRequest, "spec is required")
			return
		}
		if _, err := contracts.ParseModsSpecJSON(req.Spec); err != nil {
			httpErr(w, http.StatusBadRequest, "spec: %v", err)
			return
		}

		// v1 side-effect: Create spec row
		specID := domaintypes.NewSpecID()
		createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
			ID:        specID,
			Name:      "",
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create spec: %v", err)
			slog.Error("create single-repo run: create spec failed", "err", err)
			return
		}

		// v1 side-effect: Create mig project with name == id
		modID := domaintypes.NewMigID()
		if _, err := st.CreateMig(r.Context(), store.CreateMigParams{
			ID:        modID,
			Name:      modID.String(),
			SpecID:    &createdSpec.ID,
			CreatedBy: req.CreatedBy,
		}); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create mig: %v", err)
			slog.Error("create single-repo run: create mig failed", "mig_id", modID, "err", err)
			return
		}

		// Create mig repo for the provided repo_url
		normalizedRepoURL := domaintypes.NormalizeRepoURL(req.RepoURL.String())
		modRepoID := domaintypes.NewMigRepoID()
		modRepo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:        modRepoID,
			MigID:     modID,
			RepoUrl:   normalizedRepoURL,
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create mig repo: %v", err)
			slog.Error("create single-repo run: create mig repo failed", "mig_id", modID, "repo_url", req.RepoURL, "err", err)
			return
		}

		// Create run
		runID := domaintypes.NewRunID()
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        runID,
			MigID:     modID,
			SpecID:    createdSpec.ID,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create run: %v", err)
			slog.Error("create single-repo run: create run failed", "run_id", runID, "err", err)
			return
		}

		// Create run_repo entry
		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			MigID:         modID,
			RunID:         run.ID,
			RepoID:        modRepo.ID,
			RepoBaseRef:   modRepo.BaseRef,
			RepoTargetRef: modRepo.TargetRef,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create run repo: %v", err)
			slog.Error("create single-repo run: create run_repo failed", "run_id", run.ID, "repo_id", modRepo.ID, "err", err)
			return
		}

		// v1 immediate start: Create repo-scoped jobs for the queued repo.
		// This ensures the run starts execution immediately.
		if err := createJobsFromSpec(r.Context(), st, run.ID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, createdSpec.Spec); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create jobs: %v", err)
			slog.Error("create single-repo run: create jobs failed", "run_id", run.ID, "repo_id", runRepo.RepoID, "err", err)
			return
		}

		// Build response with run_id, mig_id, and spec_id.
		resp := struct {
			RunID  domaintypes.RunID  `json:"run_id"`
			MigID  domaintypes.MigID  `json:"mig_id"`
			SpecID domaintypes.SpecID `json:"spec_id"`
		}{
			RunID:  run.ID,
			MigID:  modID,
			SpecID: createdSpec.ID,
		}

		// Publish queued event to SSE hub for the run
		if eventsService != nil {
			// Build a minimal run summary for the event using modsapi.RunSummary
			// (matching the event structure expected by eventsService.PublishRun)
			summary := modsapi.RunSummary{
				RunID:      run.ID,
				State:      modsapi.RunStatusFromStore(run.Status),
				Submitter:  "",
				Repository: modRepo.RepoUrl,
				Metadata: map[string]string{
					"repo_base_ref":   modRepo.BaseRef,
					"repo_target_ref": modRepo.TargetRef,
				},
				CreatedAt: timeOrZero(run.CreatedAt),
				UpdatedAt: time.Now().UTC(),
				Stages:    make(map[domaintypes.JobID]modsapi.StageStatus),
			}
			if err := eventsService.PublishRun(r.Context(), run.ID, summary); err != nil {
				slog.Error("create single-repo run: publish run event failed", "run_id", run.ID, "err", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create single-repo run: encode response failed", "err", err)
		}

		slog.Info("single-repo run created",
			"run_id", run.ID,
			"mig_id", modID.String(),
			"spec_id", createdSpec.ID,
			"repo_id", runRepo.RepoID,
			"repo_url", req.RepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
		)
	}
}
