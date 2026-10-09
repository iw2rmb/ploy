package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// NOTE: This file uses KSUID-backed string IDs for runs and jobs.
// Run and job IDs are generated using domaintypes.NewRunID() and domaintypes.NewJobID().
// UUID parsing is no longer performed for run/job IDs; they are treated as opaque strings.

// Migs run handlers implement the Migs-style run status surface (RunSummary)
// and job materialization helpers.

// getRunStatusHandler returns an HTTP handler that fetches run status by ID.
//
// Endpoint: GET /v1/runs/{run_id}/status
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
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run status")
		if !ok {
			return
		}

		// Build RunSummary response with Stages and Artifacts.
		runState, convErr := migsapi.RunStatusFromDomain(run.Status)
		if convErr != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to convert run status: %v", convErr)
			slog.Error("get run status: invalid run status", "run_id", run.ID, "status", run.Status, "err", convErr)
			return
		}

		var (
			repoURL  string
			repoBase string
		)
		repoBase = run.RepoBaseRef
		if repo, err := st.GetRepo(r.Context(), run.RepoID); err == nil {
			repoURL = repo.Url
		}

		summary := migsapi.RunSummary{
			RunID:      run.ID,
			State:      runState,
			Submitter:  "",
			Repository: repoURL,
			Metadata: map[string]string{
				"repo_id":           run.RepoID.String(),
				"repo_base_ref":     repoBase,
				"source_commit_sha": run.SourceCommitSha,
			},
			CreatedAt: timeOrZero(run.CreatedAt),
			UpdatedAt: time.Now().UTC(),
			Stages:    make(map[domaintypes.JobID]migsapi.StageStatus),
		}

		// Surface gate summary and resume metadata from runs.stats if present.
		// Gate summary exposes gate health without requiring raw artifact inspection.
		// Resume metadata (resume_count, last_resumed_at) tracks resume history.
		if len(run.Stats) > 0 && json.Valid(run.Stats) {
			var stats domaintypes.RunStats
			if err := json.Unmarshal(run.Stats, &stats); err == nil {
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
			serverError(w, "get run status", "list jobs", err, "run_id", run.ID)
			return
		}
		for _, job := range jobs {
			jobIDStr := job.ID.String()
			s, convErr := migsapi.StageStatusFromDomain(job.Status)
			if convErr != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to convert stage status for job %s: %v", job.ID, convErr)
				slog.Error("get run status: invalid stage status", "run_id", run.ID, "job_id", job.ID, "status", job.Status, "err", convErr)
				return
			}
			artMap := make(map[string]string)
			bundles, err := listArtifactBundlesByJob(r.Context(), st, job)
			if err != nil {
				serverError(w, "get run status", "list artifacts", err, "run_id", run.ID, "job_id", jobIDStr)
				return
			}
			for _, b := range bundles {
				name := "artifact"
				if b.Name != nil && strings.TrimSpace(*b.Name) != "" {
					name = strings.TrimSpace(*b.Name)
				}
				if b.Cid != nil && strings.TrimSpace(*b.Cid) != "" && artMap[name] == "" {
					artMap[name] = strings.TrimSpace(*b.Cid)
				}
			}

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing chain semantics.
			summary.Stages[job.ID] = migsapi.StageStatus{
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
				NextID:      job.NextID,
			}
		}

		writeJSON(w, http.StatusOK, summary)
	}
}

func planJobsFromSpec(spec []byte) ([]store.JobPlan, error) {
	migsSpec, err := contracts.ParseMigSpecJSON(spec)
	if err != nil {
		return nil, fmt.Errorf("parse migs spec: %w", err)
	}
	type draft struct {
		name      string
		jobType   domaintypes.JobType
		jobImage  string
		stepName  string
		stepIndex *int
		gateCycle string
	}

	buildGateDisabled := migsSpec.BuildGate != nil && migsSpec.BuildGate.Disabled
	drafts := make([]draft, 0, len(migsSpec.Steps)+2)
	if !buildGateDisabled {
		drafts = append(drafts, draft{name: "pre-gate", jobType: domaintypes.JobTypePreGate, gateCycle: "pre-gate"})
	}

	if len(migsSpec.Steps) > 1 {
		for i, mig := range migsSpec.Steps {
			jobImage := ""
			if mig.Image.Universal != "" {
				jobImage = strings.TrimSpace(mig.Image.Universal)
			}
			drafts = append(drafts, draft{
				name:     fmt.Sprintf("mig-%d", i),
				jobType:  domaintypes.JobTypeMig,
				jobImage: jobImage,
				stepName: mig.Name,
				stepIndex: func() *int {
					v := i
					return &v
				}(),
			})
		}
	} else {
		migImage := ""
		stepName := ""
		if len(migsSpec.Steps) > 0 {
			if migsSpec.Steps[0].Image.Universal != "" {
				migImage = strings.TrimSpace(migsSpec.Steps[0].Image.Universal)
			}
			stepName = migsSpec.Steps[0].Name
		}
		drafts = append(drafts, draft{
			name:     "mig-0",
			jobType:  domaintypes.JobTypeMig,
			jobImage: migImage,
			stepName: stepName,
			stepIndex: func() *int {
				v := 0
				return &v
			}(),
		})
	}
	if !buildGateDisabled {
		drafts = append(drafts, draft{name: "post-gate", jobType: domaintypes.JobTypePostGate, gateCycle: "post-gate"})
	}

	planned := make([]store.JobPlan, 0, len(drafts))
	for _, d := range drafts {
		var meta *contracts.JobMeta
		if d.stepName != "" {
			meta = contracts.NewMigJobMetaWithStepName(d.stepName)
		} else {
			meta = contracts.NewMigJobMeta()
		}
		meta.MigStepIndex = d.stepIndex
		meta.GateCycleName = strings.TrimSpace(d.gateCycle)
		metaBytes, err := contracts.MarshalJobMeta(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal job %q meta: %w", d.name, err)
		}
		planned = append(planned, store.JobPlan{
			Name:     d.name,
			JobType:  d.jobType,
			JobImage: d.jobImage,
			Meta:     metaBytes,
		})
	}
	return planned, nil
}

// helpers
func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Unix(0, 0).UTC()
}
