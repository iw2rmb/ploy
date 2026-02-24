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
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type runJobCreator interface {
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
}

// NOTE: This file uses KSUID-backed string IDs for runs and jobs.
// Run and job IDs are generated using domaintypes.NewRunID() and domaintypes.NewJobID().
// UUID parsing is no longer performed for run/job IDs; they are treated as opaque strings.

// Migs run handlers implement the Migs-style run status surface (RunSummary)
// and job materialization helpers.

// getRunStatusHandler returns an HTTP handler that fetches run status by ID.
//
// Endpoint: GET /v1/runs/{id}/status
// Response: 200 OK with RunSummary body (canonical schema, no wrapper types)
//
// Canonical contract (see docs/migs-lifecycle.md § 2.1):
//   - Returns RunSummary directly as JSON root (no envelope or wrapper types).
//   - HTTP 200 on success; 404 if run not found.
//   - run_id is a KSUID string (27 characters).
//   - stages map is keyed by job ID (KSUID), not job name; use next_id links for ordering.
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
func getRunStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runID, err := parseParam[domaintypes.RunID](r, "id")
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
		runState := modsapi.RunStatusFromStore(run.Status)

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

			mr, err := st.GetMigRepo(r.Context(), rr.RepoID)
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
			s := modsapi.StageStatusFromStore(job.Status)
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

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing chain semantics.
			summary.Stages[job.ID] = modsapi.StageStatus{
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
				NextID:      job.NextID,
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

type plannedJob struct {
	ID       domaintypes.JobID
	Name     string
	JobType  string
	JobImage string
	Status   store.JobStatus
	StepName string
	NextID   *domaintypes.JobID
}

// createJobsFromSpec parses the run spec and creates an explicit next_id-linked job chain.
// Queue semantics are head-only: the first job is Queued, all successors are Created.
func createJobsFromSpec(ctx context.Context, st runJobCreator, runID domaintypes.RunID, repoID domaintypes.MigRepoID, repoBaseRef string, attempt int32, spec []byte) error {
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		return fmt.Errorf("parse migs spec: %w", err)
	}

	type draft struct {
		name     string
		jobType  string
		jobImage string
		stepName string
	}
	drafts := []draft{{name: "pre-gate", jobType: "pre_gate"}}

	if modsSpec.IsMultiStep() {
		for i, mod := range modsSpec.Steps {
			jobImage := ""
			if mod.Image.Universal != "" {
				jobImage = strings.TrimSpace(mod.Image.Universal)
			}
			drafts = append(drafts, draft{
				name:     fmt.Sprintf("mod-%d", i),
				jobType:  "mod",
				jobImage: jobImage,
				stepName: mod.Name,
			})
		}
	} else {
		modImage := ""
		stepName := ""
		if len(modsSpec.Steps) > 0 {
			if modsSpec.Steps[0].Image.Universal != "" {
				modImage = strings.TrimSpace(modsSpec.Steps[0].Image.Universal)
			}
			stepName = modsSpec.Steps[0].Name
		}
		drafts = append(drafts, draft{
			name:     "mod-0",
			jobType:  "mod",
			jobImage: modImage,
			stepName: stepName,
		})
	}
	drafts = append(drafts, draft{name: "post-gate", jobType: "post_gate"})

	planned := make([]plannedJob, 0, len(drafts))
	for i, d := range drafts {
		status := store.JobStatusCreated
		if i == 0 {
			status = store.JobStatusQueued
		}
		planned = append(planned, plannedJob{
			ID:       domaintypes.NewJobID(),
			Name:     d.name,
			JobType:  d.jobType,
			JobImage: d.jobImage,
			Status:   status,
			StepName: d.stepName,
		})
	}
	for i := range planned {
		if i+1 < len(planned) {
			nextID := planned[i+1].ID
			planned[i].NextID = &nextID
		}
	}

	for i := range planned {
		if err := createPlannedJob(ctx, st, runID, repoID, repoBaseRef, attempt, planned[i]); err != nil {
			return fmt.Errorf("create job %q: %w", planned[i].Name, err)
		}
	}
	return nil
}

func createPlannedJob(ctx context.Context, st runJobCreator, runID domaintypes.RunID, repoID domaintypes.MigRepoID, repoBaseRef string, attempt int32, planned plannedJob) error {
	// Build job metadata with step name for mod jobs.
	var meta *contracts.JobMeta
	if planned.StepName != "" {
		meta = contracts.NewModJobMetaWithStepName(planned.StepName)
	} else {
		meta = contracts.NewModJobMeta()
	}
	metaBytes, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return fmt.Errorf("marshal job meta: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          planned.ID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        planned.Name,
		Status:      planned.Status,
		JobType:     planned.JobType,
		JobImage:    planned.JobImage,
		NextID:      planned.NextID,
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
