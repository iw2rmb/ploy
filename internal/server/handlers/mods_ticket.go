package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// NOTE: This file uses KSUID-backed string IDs for runs and jobs.
// Run and job IDs are generated using domaintypes.NewRunID() and domaintypes.NewJobID().
// UUID parsing is no longer performed for run/job IDs; they are treated as opaque strings.

// Mods  run handlers implement the /v1/mods facade:  run submission from
// repo/spec,  run status as mods-style RunSummary, and stage/run_step
// materialization for single- and multi-step runs.

// submitRunHandler returns an HTTP handler that submits a new run (mods run).
//
// Endpoint: POST /v1/mods
// Request:  RunSubmitRequest {repo_url, base_ref, target_ref?, spec?, created_by?}
// Response: 201 Created with RunSummary body (canonical schema, no wrapper types)
//
// Canonical contract (see docs/mods-lifecycle.md § 2.1):
//   - Returns RunSummary directly as JSON root (no envelope or wrapper types).
//   - HTTP 201 on success; no 202 or other legacy status codes.
//   - run_id is the execution run ID (KSUID string, 27 characters).
//   - stages map is keyed by job ID (KSUID), not job name.
//
// The handler now creates a **batch parent run + run_repo entry** and then
// starts execution for that repo using the same batch machinery that powers
// multi-repo runs (BatchRepoStarter). This ensures there is a single path for
// creating execution runs and jobs for both single- and multi-repo workflows.
func submitRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req struct {
			RepoURL domaintypes.RepoURL `json:"repo_url"`
			BaseRef domaintypes.GitRef  `json:"base_ref"`
			// TargetRef is optional; when omitted, downstream components derive a default
			// branch name when an MR is actually created (using the run name when set,
			// otherwise the DB run ID).
			TargetRef *domaintypes.GitRef `json:"target_ref,omitempty"`
			Spec      *json.RawMessage    `json:"spec,omitempty"`
			CreatedBy *string             `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		// When JSON fields are omitted, domain types remain at zero value and need validation.
		if err := req.RepoURL.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
			return
		}
		if req.TargetRef != nil {
			if err := req.TargetRef.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
				return
			}
		}
		// Prepare spec (default to empty JSON object if not provided).
		spec := []byte("{}")
		if req.Spec != nil && len(*req.Spec) > 0 {
			spec = *req.Spec
		}
		if _, err := contracts.ParseModsSpecJSON(spec); err != nil {
			http.Error(w, fmt.Sprintf("spec: %v", err), http.StatusBadRequest)
			return
		}

		targetRef := ""
		if req.TargetRef != nil {
			targetRef = req.TargetRef.String()
		}

		specID := domaintypes.NewSpecID().String()
		createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
			ID:        specID,
			Name:      "",
			Spec:      spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create spec: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create spec failed", "err", err)
			return
		}

		// v1 entrypoint: `ploy run` creates a mod project as a side-effect; mod name == mod id.
		modID := domaintypes.NewModID().String()
		if _, err := st.CreateMod(r.Context(), store.CreateModParams{
			ID:        modID,
			Name:      modID,
			SpecID:    &createdSpec.ID,
			CreatedBy: req.CreatedBy,
		}); err != nil {
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create mod failed", "mod_id", modID, "err", err)
			return
		}

		modRepoID := domaintypes.NewModRepoID().String()
		modRepo, err := st.CreateModRepo(r.Context(), store.CreateModRepoParams{
			ID:        modRepoID,
			ModID:     modID,
			RepoUrl:   req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: targetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create mod repo: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create mod repo failed", "mod_id", modID, "repo_url", req.RepoURL, "err", err)
			return
		}

		runID := domaintypes.NewRunID().String()
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        runID,
			ModID:     modID,
			SpecID:    createdSpec.ID,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create run failed", "run_id", runID, "err", err)
			return
		}

		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			ModID:         modID,
			RunID:         run.ID,
			RepoID:        modRepo.ID,
			RepoBaseRef:   modRepo.BaseRef,
			RepoTargetRef: modRepo.TargetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run repo: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create run_repo failed", "run_id", run.ID, "repo_id", modRepo.ID, "err", err)
			return
		}

		if err := createJobsFromSpec(r.Context(), st, run.ID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, createdSpec.Spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create jobs failed", "run_id", run.ID, "repo_id", runRepo.RepoID, "err", err)
			return
		}

		summary := modsapi.RunSummary{
			RunID:      domaintypes.RunID(run.ID),
			State:      modsapi.RunStatusFromStore(run.Status),
			Submitter:  "",
			Repository: modRepo.RepoUrl,
			Metadata: map[string]string{
				"repo_base_ref":   runRepo.RepoBaseRef,
				"repo_target_ref": runRepo.RepoTargetRef,
			},
			CreatedAt: timeOrZero(run.CreatedAt),
			UpdatedAt: timeOrZero(run.CreatedAt),
			Stages:    make(map[string]modsapi.StageStatus),
		}

		// Publish queued event to SSE hub for the execution run.
		if eventsService != nil {
			if err := eventsService.PublishRun(r.Context(), summary.RunID, summary); err != nil {
				slog.Error("submit run: publish run event failed", "run_id", summary.RunID, "err", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// Encode RunSummary directly — canonical schema for POST /v1/mods.
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("submit run: encode response failed", "err", err)
		}

		slog.Info("run submitted",
			"run_id", run.ID,
			"repo_id", runRepo.RepoID,
			"repo_url", req.RepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
			"state", summary.State,
		)
	}
}

// getRunStatusHandler returns an HTTP handler that fetches run status by ID.
//
// Endpoint: GET /v1/runs/{id}/status
// Response: 200 OK with RunSummary body (canonical schema, no wrapper types)
//
// Canonical contract (see docs/mods-lifecycle.md § 2.1):
//   - Returns RunSummary directly as JSON root (no envelope or wrapper types).
//   - HTTP 200 on success; 404 if run not found.
//   - run_id is a KSUID string (27 characters).
//   - stages map is keyed by job ID (KSUID), not job name; use step_index for ordering.
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
func getRunStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Fetch run using string ID directly (no UUID parsing needed).
		run, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("get run status: fetch run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Build RunSummary response with Stages and Artifacts.
		// Use conversion helper to map store.RunStatus to modsapi.RunState.
		runState := modsapi.RunStatusFromStore(run.Status)

		var (
			repoURL    string
			repoBase   string
			repoTarget string
		)
		runRepos, err := st.ListRunReposByRun(r.Context(), run.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("get run status: list run repos failed", "run_id", run.ID, "err", err)
			return
		}
		if len(runRepos) > 0 {
			rr := runRepos[0]
			repoBase = rr.RepoBaseRef
			repoTarget = rr.RepoTargetRef

			mr, err := st.GetModRepo(r.Context(), rr.RepoID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
				slog.Error("get run status: get repo failed", "run_id", run.ID, "repo_id", rr.RepoID, "err", err)
				return
			}
			repoURL = mr.RepoUrl
		}

		// run.ID is now a string (KSUID). Construct RunSummary with RunID.
		summary := modsapi.RunSummary{
			RunID:      domaintypes.RunID(run.ID),
			State:      runState,
			Submitter:  "",
			Repository: repoURL,
			Metadata:   map[string]string{"repo_base_ref": repoBase, "repo_target_ref": repoTarget},
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}

		// Surface MR URL, gate summary, and resume metadata from runs.stats if present.
		// Node stores MR URL under stats.metadata.mr_url and gate data under stats.gate.
		// Gate summary exposes gate health without requiring raw artifact inspection.
		// Resume metadata (resume_count, last_resumed_at) tracks resume history.
		if len(run.Stats) > 0 && json.Valid(run.Stats) {
			var stats domaintypes.RunStats
			if err := json.Unmarshal(run.Stats, &stats); err == nil {
				if mr := stats.MRURL(); mr != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["mr_url"] = mr
				}
				// Extract gate summary for quick gate health visibility.
				if gateSummary := stats.GateSummary(); gateSummary != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["gate_summary"] = gateSummary
				}
				// Extract resume metadata so clients can see resume history.
				if rc := stats.ResumeCount(); rc > 0 {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["resume_count"] = strconv.Itoa(rc)
				}
				if lra := stats.LastResumedAt(); lra != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["last_resumed_at"] = lra
				}
			}
		}

		// Load jobs and their artifacts using string run ID.
		jobs, err := st.ListJobsByRun(r.Context(), run.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			slog.Error("get run status: list jobs failed", "run_id", run.ID, "err", err)
			return
		}
		for _, job := range jobs {
			// Use conversion helper to map store.JobStatus -> modsapi.StageState
			s := modsapi.StageStatusFromStore(job.Status)
			artMap := make(map[string]string)
			// job.ID and run.ID are now strings (KSUID).
			bundles, err := st.ListArtifactBundlesByRunAndJob(r.Context(), store.ListArtifactBundlesByRunAndJobParams{RunID: run.ID, JobID: &job.ID})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list artifacts: %v", err), http.StatusInternalServerError)
				slog.Error("get run status: list artifacts failed", "run_id", run.ID, "job_id", job.ID, "err", err)
				return
			}
			for _, b := range bundles {
				name := "artifact"
				if b.Name != nil && strings.TrimSpace(*b.Name) != "" {
					name = strings.TrimSpace(*b.Name)
				}
				if b.Cid != nil && strings.TrimSpace(*b.Cid) != "" {
					artMap[name] = strings.TrimSpace(*b.Cid)
				}
			}

			// Use job's step_index directly for ordering (float, but expose as int for API).
			stepIndex := int(job.StepIndex)

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing StepIndex semantics.
			// job.ID is now a string (KSUID).
			summary.Stages[job.ID] = modsapi.StageStatus{
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
				StepIndex:   stepIndex,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Encode RunSummary directly — no wrapper type.
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("get run status: encode response failed", "err", err)
		}
	}
}

// createJobsFromSpec parses the run spec and creates jobs for the execution pipeline.
// It uses the canonical contracts.ParseModsSpecJSON parser for structured validation.
// Jobs are created with float step_index for ordered execution with dynamic insertion support.
//
// Default job layout:
//   - pre-gate  (step_index=1000): Pre-mod validation/gate
//   - mod-0     (step_index=2000): First mod execution
//   - post-gate (step_index=3000): Post-mod validation
//
// For multi-step runs (mods[] array), creates one mod job per entry.
// Healing jobs can be inserted dynamically between existing jobs using midpoint calculation.
func createJobsFromSpec(ctx context.Context, st store.Store, runID string, repoID string, repoBaseRef string, attempt int32, spec []byte) error {
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		return fmt.Errorf("parse mods spec: %w", err)
	}

	if modsSpec.IsMultiStep() {
		// v1 job queueing rules: first job is Queued, rest are Created (roadmap/v1/statuses.md:52-55).
		if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusQueued); err != nil {
			return fmt.Errorf("create pre-gate job: %w", err)
		}

		for i, mod := range modsSpec.Mods {
			modImage := ""
			if mod.Image.Universal != "" {
				modImage = strings.TrimSpace(mod.Image.Universal)
			}
			jobName := fmt.Sprintf("mod-%d", i)
			stepIndex := domaintypes.StepIndex(2000 + i*1000)
			if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, jobName, "mod", stepIndex, modImage, store.JobStatusCreated); err != nil {
				return fmt.Errorf("create mod job %d: %w", i, err)
			}
		}

		postGateIndex := domaintypes.StepIndex(2000 + len(modsSpec.Mods)*1000)
		if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "post-gate", "post_gate", postGateIndex, "", store.JobStatusCreated); err != nil {
			return fmt.Errorf("create post-gate job: %w", err)
		}
		return nil
	}

	modImage := ""
	if modsSpec.Image.Universal != "" {
		modImage = strings.TrimSpace(modsSpec.Image.Universal)
	}
	return createSingleModJob(ctx, st, runID, repoID, repoBaseRef, attempt, modImage)
}

func createSingleModJob(ctx context.Context, st store.Store, runID string, repoID string, repoBaseRef string, attempt int32, modImage string) error {
	// v1 job queueing rules: first job is Queued, rest are Created (roadmap/v1/statuses.md:52-55).
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusQueued); err != nil {
		return fmt.Errorf("create pre-gate job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "mod-0", "mod", domaintypes.StepIndex(2000), modImage, store.JobStatusCreated); err != nil {
		return fmt.Errorf("create mod job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "post-gate", "post_gate", domaintypes.StepIndex(3000), "", store.JobStatusCreated); err != nil {
		return fmt.Errorf("create post-gate job: %w", err)
	}
	return nil
}

func createJobWithIndex(ctx context.Context, st store.Store, runID string, repoID string, repoBaseRef string, attempt int32, name string, modType string, stepIndex domaintypes.StepIndex, modImage string, status store.JobStatus) error {
	jobID := domaintypes.NewJobID()
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		ID:          string(jobID),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      status,
		ModType:     modType,
		ModImage:    modImage,
		StepIndex:   stepIndex.Float64(),
		Meta:        []byte(`{}`),
	})
	return err
}

// helpers
func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Unix(0, 0).UTC()
}
