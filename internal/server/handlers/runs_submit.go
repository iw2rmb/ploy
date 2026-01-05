package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// createSingleRepoRunHandler submits a single-repo run and immediately starts execution.
// Endpoint: POST /v1/runs
// Request: {repo_url, base_ref, target_ref, spec}
// Response: 201 Created with {run_id, mod_id, spec_id}
//
// v1 contract (roadmap/v1/api.md:104-128):
// - Submits a single-repo run via POST /v1/runs.
// - Creates a mod project as a side-effect; mod name == mod id.
// - Creates an initial spec row and sets mods.spec_id.
// - Creates a mod repo row for the provided repo_url.
// - Creates a run and starts execution immediately.
//
// This handler replaces the previous POST /v1/mods endpoint for run submission.
// The old submitRunHandler in mods_ticket.go remains for backward compatibility
// or will be removed once all clients migrate to POST /v1/runs.
func createSingleRepoRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req struct {
			RepoURL   domaintypes.RepoURL `json:"repo_url"`
			BaseRef   domaintypes.GitRef  `json:"base_ref"`
			TargetRef domaintypes.GitRef  `json:"target_ref"`
			Spec      json.RawMessage     `json:"spec"`
			CreatedBy *string             `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		if err := req.RepoURL.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
			return
		}

		// Validate spec (cannot be empty for single-repo run submission)
		if len(req.Spec) == 0 {
			http.Error(w, "spec is required", http.StatusBadRequest)
			return
		}
		if _, err := contracts.ParseModsSpecJSON(req.Spec); err != nil {
			http.Error(w, fmt.Sprintf("spec: %v", err), http.StatusBadRequest)
			return
		}

		// v1 side-effect: Create spec row
		specID := domaintypes.NewSpecID().String()
		createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
			ID:        specID,
			Name:      "",
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create spec: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create spec failed", "err", err)
			return
		}

		// v1 side-effect: Create mod project with name == id
		modID := domaintypes.NewModID().String()
		if _, err := st.CreateMod(r.Context(), store.CreateModParams{
			ID:        modID,
			Name:      modID,
			SpecID:    &createdSpec.ID,
			CreatedBy: req.CreatedBy,
		}); err != nil {
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create mod failed", "mod_id", modID, "err", err)
			return
		}

		// Create mod repo for the provided repo_url
		modRepoID := domaintypes.NewModRepoID().String()
		modRepo, err := st.CreateModRepo(r.Context(), store.CreateModRepoParams{
			ID:        modRepoID,
			ModID:     modID,
			RepoUrl:   req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create mod repo: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create mod repo failed", "mod_id", modID, "repo_url", req.RepoURL, "err", err)
			return
		}

		// Create run
		runID := domaintypes.NewRunID().String()
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        runID,
			ModID:     modID,
			SpecID:    createdSpec.ID,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create run failed", "run_id", runID, "err", err)
			return
		}

		// Create run_repo entry
		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			ModID:         modID,
			RunID:         run.ID,
			RepoID:        modRepo.ID,
			RepoBaseRef:   modRepo.BaseRef,
			RepoTargetRef: modRepo.TargetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run repo: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create run_repo failed", "run_id", run.ID, "repo_id", modRepo.ID, "err", err)
			return
		}

		// v1 immediate start: Create repo-scoped jobs for the queued repo.
		// This ensures the run starts execution immediately (roadmap/v1/scope.md:51).
		if err := createJobsFromSpec(r.Context(), st, run.ID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, createdSpec.Spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
			slog.Error("create single-repo run: create jobs failed", "run_id", run.ID, "repo_id", runRepo.RepoID, "err", err)
			return
		}

		// Build response per v1 contract (roadmap/v1/api.md:123-128)
		resp := struct {
			RunID  string `json:"run_id"`
			ModID  string `json:"mod_id"`
			SpecID string `json:"spec_id"`
		}{
			RunID:  run.ID,
			ModID:  modID,
			SpecID: createdSpec.ID,
		}

		// Publish queued event to SSE hub for the run
		if eventsService != nil {
			// Build a minimal run summary for the event using modsapi.RunSummary
			// (matching the event structure expected by eventsService.PublishRun)
			summary := modsapi.RunSummary{
				RunID:      domaintypes.RunID(run.ID),
				State:      modsapi.RunStatusFromStore(run.Status),
				Submitter:  "",
				Repository: modRepo.RepoUrl,
				Metadata: map[string]string{
					"repo_base_ref":   modRepo.BaseRef,
					"repo_target_ref": modRepo.TargetRef,
				},
				CreatedAt: timeOrZero(run.CreatedAt),
				UpdatedAt: time.Now().UTC(),
				Stages:    make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishRun(r.Context(), domaintypes.RunID(run.ID), summary); err != nil {
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
			"mod_id", modID,
			"spec_id", createdSpec.ID,
			"repo_id", runRepo.RepoID,
			"repo_url", req.RepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
		)
	}
}
