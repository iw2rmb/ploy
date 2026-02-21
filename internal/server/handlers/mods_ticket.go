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
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type runJobCreator interface {
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
}

// NOTE: This file uses KSUID-backed string IDs for runs and jobs.
// Run and job IDs are generated using domaintypes.NewRunID() and domaintypes.NewJobID().
// UUID parsing is no longer performed for run/job IDs; they are treated as opaque strings.

// Mods run handlers implement the Mods-style run status surface (RunSummary)
// and job materialization helpers.

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
		runID, err := ParseRunIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Fetch run.
		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("get run status: fetch run failed", "run_id", runID.String(), "err", err)
			return
		}

		// Build RunSummary response with Stages and Artifacts.
		// Use conversion helper to map store.RunStatus to modsapi.RunState.
		runState := RunStatusFromStore(run.Status)

		var (
			repoURL    string
			repoBase   string
			repoTarget string
		)
		runRepos, err := st.ListRunReposByRun(r.Context(), run.ID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list run repos: %v", err)
			slog.Error("get run status: list run repos failed", "run_id", run.ID, "err", err)
			return
		}
		if len(runRepos) > 0 {
			rr := runRepos[0]
			repoBase = rr.RepoBaseRef
			repoTarget = rr.RepoTargetRef

			mr, err := st.GetModRepo(r.Context(), rr.RepoID)
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("get run status: get repo failed", "run_id", run.ID, "repo_id", rr.RepoID, "err", err)
				return
			}
			repoURL = mr.RepoUrl
		}

		summary := modsapi.RunSummary{
			RunID:      run.ID,
			State:      runState,
			Submitter:  "",
			Repository: repoURL,
			Metadata:   map[string]string{"repo_base_ref": repoBase, "repo_target_ref": repoTarget},
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[domaintypes.JobID]modsapi.StageStatus),
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
			httpErr(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("get run status: list jobs failed", "run_id", run.ID, "err", err)
			return
		}
		for _, job := range jobs {
			jobIDStr := job.ID.String()
			// Use conversion helper to map store.JobStatus -> modsapi.StageState
			s := StageStatusFromStore(job.Status)
			artMap := make(map[string]string)
			bundles, err := st.ListArtifactBundlesMetaByRunAndJob(r.Context(), store.ListArtifactBundlesMetaByRunAndJobParams{RunID: run.ID, JobID: &job.ID})
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to list artifacts: %v", err)
				slog.Error("get run status: list artifacts failed", "run_id", run.ID, "job_id", jobIDStr, "err", err)
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

			// Use job's step_index directly without lossy int cast.
			// Validate that step_index is finite (reject NaN/Inf).
			stepIndex := job.StepIndex
			if !stepIndex.Valid() {
				httpErr(w, http.StatusInternalServerError, "invalid step_index for job %s", jobIDStr)
				slog.Error("get run status: invalid step_index", "run_id", run.ID, "job_id", jobIDStr, "step_index", float64(job.StepIndex))
				return
			}

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing StepIndex semantics.
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
func createJobsFromSpec(ctx context.Context, st runJobCreator, runID domaintypes.RunID, repoID domaintypes.ModRepoID, repoBaseRef string, attempt int32, spec []byte) error {
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		return fmt.Errorf("parse mods spec: %w", err)
	}

	if modsSpec.IsMultiStep() {
		// v1 job queueing rules: first job is Queued, rest are Created.
		if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusQueued, ""); err != nil {
			return fmt.Errorf("create pre-gate job: %w", err)
		}

		for i, mod := range modsSpec.Steps {
			modImage := ""
			if mod.Image.Universal != "" {
				modImage = strings.TrimSpace(mod.Image.Universal)
			}
			jobName := fmt.Sprintf("mod-%d", i)
			stepIndex := domaintypes.StepIndex(2000 + i*1000)
			// Pass the user-defined step name for CLI display in --follow mode.
			stepName := mod.Name
			if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, jobName, "mod", stepIndex, modImage, store.JobStatusCreated, stepName); err != nil {
				return fmt.Errorf("create mod job %d: %w", i, err)
			}
		}

		postGateIndex := domaintypes.StepIndex(2000 + len(modsSpec.Steps)*1000)
		if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "post-gate", "post_gate", postGateIndex, "", store.JobStatusCreated, ""); err != nil {
			return fmt.Errorf("create post-gate job: %w", err)
		}
		return nil
	}

	modImage := ""
	stepName := ""
	if len(modsSpec.Steps) > 0 {
		if modsSpec.Steps[0].Image.Universal != "" {
			modImage = strings.TrimSpace(modsSpec.Steps[0].Image.Universal)
		}
		stepName = modsSpec.Steps[0].Name
	}
	return createSingleModJob(ctx, st, runID, repoID, repoBaseRef, attempt, modImage, stepName)
}

func createSingleModJob(ctx context.Context, st runJobCreator, runID domaintypes.RunID, repoID domaintypes.ModRepoID, repoBaseRef string, attempt int32, modImage string, stepName string) error {
	// v1 job queueing rules: first job is Queued, rest are Created.
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "pre-gate", "pre_gate", domaintypes.StepIndex(1000), "", store.JobStatusQueued, ""); err != nil {
		return fmt.Errorf("create pre-gate job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "mod-0", "mod", domaintypes.StepIndex(2000), modImage, store.JobStatusCreated, stepName); err != nil {
		return fmt.Errorf("create mod job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, repoID, repoBaseRef, attempt, "post-gate", "post_gate", domaintypes.StepIndex(3000), "", store.JobStatusCreated, ""); err != nil {
		return fmt.Errorf("create post-gate job: %w", err)
	}
	return nil
}

func createJobWithIndex(ctx context.Context, st runJobCreator, runID domaintypes.RunID, repoID domaintypes.ModRepoID, repoBaseRef string, attempt int32, name string, modType string, stepIndex domaintypes.StepIndex, modImage string, status store.JobStatus, modsStepName string) error {
	jobID := domaintypes.NewJobID()

	// Build job metadata with step name for mod jobs.
	var meta *contracts.JobMeta
	if modsStepName != "" {
		meta = contracts.NewModJobMetaWithStepName(modsStepName)
	} else {
		meta = contracts.NewModJobMeta()
	}
	metaBytes, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return fmt.Errorf("marshal job meta: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      status,
		ModType:     modType,
		ModImage:    modImage,
		StepIndex:   stepIndex,
		Meta:        metaBytes,
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
