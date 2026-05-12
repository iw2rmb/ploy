package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// listRunRepoJobsHandler returns jobs for a specific repo execution within a run.
// GET /v1/runs/{run_id}/repos/{repo_id}/jobs
// Query params: ?attempt=N (optional, defaults to current attempt)
func listRunRepoJobsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		repoID, ok := parseRequiredPathIDOrWriteError[domaintypes.RepoID](w, r, "repo_id")
		if !ok {
			return
		}

		rr, ok := getRunRepoOrFail(w, r, st, runID, repoID, "list run repo jobs")
		if !ok {
			return
		}

		// Use attempt from query param if provided, otherwise use current attempt.
		attempt := rr.Attempt
		if q := r.URL.Query().Get("attempt"); q != "" {
			parsed, err := strconv.ParseInt(q, 10, 32)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "invalid attempt parameter")
				return
			}
			attempt = int32(parsed)
		}

		jobs, ok := listJobsForRunRepoOrFail(w, r, st, runID, repoID, attempt, "list run repo jobs")
		if !ok {
			return
		}
		jobs = jobchain.Order(
			jobs,
			func(job store.Job) domaintypes.JobID { return job.ID },
			func(job store.Job) *domaintypes.JobID { return job.NextID },
		)

		resp := migsapi.ListRunRepoJobsResponse{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
			Jobs:    make([]migsapi.RunRepoJob, 0, len(jobs)),
		}

		for _, job := range jobs {
			resp.Jobs = append(resp.Jobs, runRepoJobFromStore(r, st, runID, job))
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// runRepoJobFromStore projects a store.Job into the API shape, applying job
// metadata, gate SBOM evidence, and timestamp conversions.
func runRepoJobFromStore(r *http.Request, st store.Store, runID domaintypes.RunID, job store.Job) migsapi.RunRepoJob {
	jr := migsapi.RunRepoJob{
		JobID:      job.ID,
		Name:       string(job.JobType),
		JobType:    job.JobType,
		JobImage:   strings.TrimSpace(job.JobImage),
		RepoShaIn:  job.RepoShaIn,
		RepoShaOut: job.RepoShaOut,
		NextID:     job.NextID,
		NodeID:     job.NodeID,
		Status:     job.Status,
		ExitCode:   job.ExitCode,
		DurationMs: job.DurationMs,
	}

	if len(job.Meta) > 0 {
		if meta, err := contracts.UnmarshalJobMeta(job.Meta); err == nil {
			if resolvedName := deriveRunRepoJobName(job, meta); resolvedName != "" {
				jr.Name = resolvedName
			}
			if meta.MigStepName != "" {
				jr.DisplayName = meta.MigStepName
			}
			if jr.BugSummary == "" && meta.GateMetadata != nil && strings.TrimSpace(meta.GateMetadata.BugSummary) != "" {
				jr.BugSummary = strings.TrimSpace(meta.GateMetadata.BugSummary)
			}
			if meta.GateMetadata != nil && meta.GateMetadata.StackGate != nil {
				if runtimeImage := strings.TrimSpace(meta.GateMetadata.StackGate.RuntimeImage); runtimeImage != "" {
					jr.JobImage = runtimeImage
				}
			}
			if meta.GateMetadata != nil {
				if exp := meta.GateMetadata.DetectedStackExpectation(); exp != nil {
					jr.Lang = exp.Language
					jr.Tooling = exp.Tool
					jr.Version = exp.Release
				}
			}
		}
	}

	if lifecycle.IsGateJobType(domaintypes.JobType(job.JobType)) {
		jr.SBOMEvidence = loadSBOMEvidence(r, st, runID, job.ID)
	}

	if job.StartedAt.Valid {
		t := job.StartedAt.Time.UTC()
		jr.StartedAt = &t
	}
	if job.FinishedAt.Valid {
		t := job.FinishedAt.Time.UTC()
		jr.FinishedAt = &t
	}
	return jr
}

func loadSBOMEvidence(r *http.Request, st store.Store, runID domaintypes.RunID, jobID domaintypes.JobID) *migsapi.RunRepoJobSBOMEvidence {
	var evidence migsapi.RunRepoJobSBOMEvidence
	hasEvidence := false

	job, err := st.GetJob(r.Context(), jobID)
	if err != nil {
		slog.Warn("list run repo jobs: load sbom evidence get job failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
		return nil
	}
	bundles, err := listArtifactBundlesByEffectiveJob(r.Context(), st, job)
	if err != nil {
		slog.Warn("list run repo jobs: load sbom artifact evidence failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
	} else {
		artifactPresent := len(bundles) > 0
		evidence.ArtifactPresent = &artifactPresent
		hasEvidence = true
	}

	sbomRows, err := listSBOMRowsByEffectiveJob(r.Context(), st, job)
	if err != nil {
		slog.Warn("list run repo jobs: load sbom package-count evidence failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
	} else {
		parsedPackageCount := len(sbomRows)
		evidence.ParsedPackageCount = &parsedPackageCount
		hasEvidence = true
	}

	if !hasEvidence {
		return nil
	}
	return &evidence
}

func deriveRunRepoJobName(job store.Job, meta *contracts.JobMeta) string {
	switch domaintypes.JobType(job.JobType) {
	case domaintypes.JobTypePreGate:
		return "pre-gate"
	case domaintypes.JobTypePostGate:
		return "post-gate"
	case domaintypes.JobTypeMig:
		if meta != nil && meta.MigStepIndex != nil {
			return fmt.Sprintf("mig-%d", *meta.MigStepIndex)
		}
		return "mig"
	default:
		return strings.TrimSpace(job.JobType.String())
	}
}
