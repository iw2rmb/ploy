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
// Request:  RunSubmitRequest {repo_url, base_ref, target_ref?, commit_sha?, spec?, created_by?}
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
			TargetRef *domaintypes.GitRef    `json:"target_ref,omitempty"`
			CommitSha *domaintypes.CommitSHA `json:"commit_sha,omitempty"`
			Spec      *json.RawMessage       `json:"spec,omitempty"`
			CreatedBy *string                `json:"created_by,omitempty"`
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
		if req.CommitSha != nil {
			if err := req.CommitSha.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("commit_sha: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Prepare spec (default to empty JSON object if not provided).
		spec := []byte("{}")
		if req.Spec != nil && len(*req.Spec) > 0 {
			spec = *req.Spec
		}

		// Convert domain types to strings for storage layer.
		// Generate KSUID-backed parent run ID using central helper.
		var commitShaStr *string
		if req.CommitSha != nil {
			s := req.CommitSha.String()
			commitShaStr = &s
		}
		targetRef := ""
		if req.TargetRef != nil {
			targetRef = req.TargetRef.String()
		}

		// 1. Create the parent batch run. This run holds the shared spec and
		//    aggregates per-repo status via run_repos. It does not have jobs.
		parentRunID := domaintypes.NewRunID()
		parentRun, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ID:        string(parentRunID),
			Name:      nil, // Optional batch name (not provided by RunSubmitRequest).
			RepoUrl:   req.RepoURL.String(),
			Spec:      spec,
			CreatedBy: req.CreatedBy,
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef.String(),
			TargetRef: targetRef,
			CommitSha: commitShaStr,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create parent run failed", "repo_url", req.RepoURL, "err", err)
			return
		}

		// 2. Attach a single repo entry to the parent run. This models the
		//    submission as a degenerate batch with exactly one repo.
		repoID := domaintypes.NewRunRepoID()
		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			ID:        string(repoID),
			RunID:     parentRunID,
			RepoUrl:   req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: targetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run repo: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: create run_repo failed", "run_id", parentRun.ID, "repo_url", req.RepoURL, "err", err)
			return
		}

		// 3. Start execution for the pending repo using the batch machinery.
		//    This creates a child execution run with jobs and links it via
		//    run_repos.execution_run_id.
		starter := NewBatchRepoStarter(st)
		started, err := starter.StartPendingRepos(r.Context(), parentRun.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to start repo execution: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: start pending repos failed", "run_id", parentRun.ID, "err", err)
			return
		}
		if started == 0 {
			http.Error(w, "failed to start repo execution", http.StatusInternalServerError)
			slog.Error("submit run: no repos started", "run_id", parentRun.ID, "repo_id", runRepo.ID)
			return
		}

		// Re-fetch the repo entry to obtain the execution_run_id.
		runRepo, err = st.GetRunRepo(r.Context(), runRepo.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to reload run repo: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: get run_repo failed after start", "run_id", parentRun.ID, "repo_id", runRepo.ID, "err", err)
			return
		}
		if runRepo.ExecutionRunID == nil || strings.TrimSpace(*runRepo.ExecutionRunID) == "" {
			http.Error(w, "run repo execution_run_id missing after start", http.StatusInternalServerError)
			slog.Error("submit run: execution_run_id missing", "run_id", parentRun.ID, "repo_id", runRepo.ID)
			return
		}

		// 4. Load the child execution run and build the canonical RunSummary
		//    for it. The execution run is the Mods run exposed to clients.
		childRunID := strings.TrimSpace(*runRepo.ExecutionRunID)
		childRun, err := st.GetRun(r.Context(), childRunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get execution run: %v", err), http.StatusInternalServerError)
			slog.Error("submit run: get execution run failed", "execution_run_id", childRunID, "err", err)
			return
		}

		summary := modsapi.RunSummary{
			RunID:      domaintypes.RunID(childRun.ID),
			State:      modsapi.RunStatusFromStore(childRun.Status),
			Submitter:  "",
			Repository: childRun.RepoUrl,
			Metadata: map[string]string{
				"repo_base_ref":   childRun.BaseRef,
				"repo_target_ref": childRun.TargetRef,
			},
			CreatedAt: timeOrZero(childRun.CreatedAt),
			UpdatedAt: timeOrZero(childRun.CreatedAt),
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
			"parent_run_id", parentRun.ID,
			"execution_run_id", summary.RunID,
			"repo_id", runRepo.ID,
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
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "run id is required", http.StatusBadRequest)
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

		// run.ID is now a string (KSUID). Construct RunSummary with RunID.
		summary := modsapi.RunSummary{
			RunID:      domaintypes.RunID(run.ID),
			State:      runState,
			Submitter:  "",
			Repository: run.RepoUrl,
			Metadata:   map[string]string{"repo_base_ref": run.BaseRef, "repo_target_ref": run.TargetRef},
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}

		// Include claiming node id when available for easier diagnostics.
		// Node IDs are now NanoID(6) strings.
		if run.NodeID != nil {
			summary.Metadata["node_id"] = *run.NodeID
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
			bundles, err := st.ListArtifactBundlesByRunAndJob(r.Context(), store.ListArtifactBundlesByRunAndJobParams{RunID: domaintypes.RunID(run.ID), JobID: &job.ID})
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
// Jobs are created with float step_index for ordered execution with dynamic insertion support.
//
// Default job layout:
//   - pre-gate  (step_index=1000): Pre-mod validation/gate
//   - mod-0     (step_index=2000): First mod execution
//   - post-gate (step_index=3000): Post-mod validation
//
// For multi-step runs (mods[] array), creates one mod job per entry.
// Healing jobs can be inserted dynamically between existing jobs using midpoint calculation.
//
// runID is a KSUID-backed domain type; job IDs are generated using domaintypes.NewJobID().
func createJobsFromSpec(ctx context.Context, st store.Store, runID domaintypes.RunID, spec []byte) error {
	// Parse spec to detect multi-step vs single-step.
	var specMap map[string]interface{}
	if len(spec) > 0 && json.Valid(spec) {
		if err := json.Unmarshal(spec, &specMap); err != nil {
			// Invalid JSON; fallback to single mod job.
			return createSingleModJob(ctx, st, runID, "")
		}
	}

	// Check for mods[] array (multi-step run).
	if mods, ok := specMap["mods"].([]interface{}); ok && len(mods) > 0 {
		// Multi-step run: create pre-gate, one job per mod, and post-gate.
		// Server-driven scheduling: first job (pre-gate) is 'pending', rest are 'created'.
		// Pre-gate job - pending (ready to be claimed immediately)
		if err := createJobWithIndex(ctx, st, runID, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusPending); err != nil {
			return fmt.Errorf("create pre-gate job: %w", err)
		}

		// Mod jobs: step_index starts at 2000, increment by 1000
		// All mod jobs start as 'created' - server will schedule them after prior job completes.
		for i, modInterface := range mods {
			modImage := ""
			if modMap, ok := modInterface.(map[string]interface{}); ok {
				if img, ok := modMap["image"].(string); ok {
					modImage = strings.TrimSpace(img)
				}
			}
			jobName := fmt.Sprintf("mod-%d", i)
			stepIndex := domaintypes.StepIndex(2000 + i*1000)
			if err := createJobWithIndex(ctx, st, runID, jobName, "mod", stepIndex, modImage, store.JobStatusCreated); err != nil {
				return fmt.Errorf("create mod job %d: %w", i, err)
			}
		}

		// Post-gate job: after all mods - starts as 'created'
		postGateIndex := domaintypes.StepIndex(2000 + len(mods)*1000)
		if err := createJobWithIndex(ctx, st, runID, "post-gate", "post_gate", postGateIndex, "", store.JobStatusCreated); err != nil {
			return fmt.Errorf("create post-gate job: %w", err)
		}
		return nil
	}

	// Single-step run: create pre-gate, single mod, and post-gate.
	modImage := ""
	if mod, ok := specMap["mod"].(map[string]interface{}); ok {
		if img, ok := mod["image"].(string); ok {
			modImage = strings.TrimSpace(img)
		}
	} else if img, ok := specMap["image"].(string); ok {
		modImage = strings.TrimSpace(img)
	}
	return createSingleModJob(ctx, st, runID, modImage)
}

// createSingleModJob creates the standard 3-job pipeline: pre-gate, mod-0, post-gate.
// Server-driven scheduling: first job (pre-gate) is 'pending', rest are 'created'.
// runID is a KSUID-backed domain type.
func createSingleModJob(ctx context.Context, st store.Store, runID domaintypes.RunID, modImage string) error {
	// Pre-gate is pending (ready to claim), others are created (wait for server to schedule).
	if err := createJobWithIndex(ctx, st, runID, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusPending); err != nil {
		return fmt.Errorf("create pre-gate job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, "mod-0", "mod", domaintypes.StepIndex(2000), modImage, store.JobStatusCreated); err != nil {
		return fmt.Errorf("create mod job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, "post-gate", "post_gate", domaintypes.StepIndex(3000), "", store.JobStatusCreated); err != nil {
		return fmt.Errorf("create post-gate job: %w", err)
	}
	return nil
}

// createJobWithIndex creates a job with the given step_index, status, and metadata.
// modType identifies the job phase ("pre_gate", "mod", "post_gate", "heal").
// status should be JobStatusPending for the first job, JobStatusCreated for others.
// runID is a KSUID-backed domain type; job ID is generated using domaintypes.NewJobID().
func createJobWithIndex(ctx context.Context, st store.Store, runID domaintypes.RunID, name, modType string, stepIndex domaintypes.StepIndex, modImage string, status store.JobStatus) error {
	// Generate KSUID-backed job ID using central helper.
	jobID := domaintypes.NewJobID()
	// Create the job with step_index and status.
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		ID:        string(jobID),
		RunID:     runID,
		Name:      name,
		Status:    status,
		ModType:   modType,
		ModImage:  modImage,
		StepIndex: stepIndex.Float64(),
		Meta:      []byte(`{}`),
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
