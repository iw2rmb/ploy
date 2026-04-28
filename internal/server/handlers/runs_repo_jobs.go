package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

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
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseRequiredPathID[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				writeHTTPError(w, http.StatusNotFound, "repo not found")
			default:
				writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("list run repo jobs: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
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

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list run repo jobs: list jobs failed", "run_id", runID.String(), "repo_id", repoID.String(), "attempt", attempt, "err", err)
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

			var meta *contracts.JobMeta

			// Extract projection fields from structured job metadata.
			if len(job.Meta) > 0 {
				meta, err = contracts.UnmarshalJobMeta(job.Meta)
				if err == nil {
					if resolvedName := deriveRunRepoJobName(job, meta); resolvedName != "" {
						jr.Name = resolvedName
					}
					if meta.MigStepName != "" {
						jr.DisplayName = meta.MigStepName
					}
					if meta.Heal != nil {
						jr.BugSummary = strings.TrimSpace(meta.Heal.BugSummary)
						jr.ActionSummary = strings.TrimSpace(meta.Heal.ActionSummary)
						jr.ErrorKind = strings.TrimSpace(meta.Heal.ErrorKind)
					}
					if jr.ActionSummary == "" && meta.ActionSummary != "" {
						jr.ActionSummary = strings.TrimSpace(meta.ActionSummary)
					}
					if jr.BugSummary == "" && meta.GateMetadata != nil && strings.TrimSpace(meta.GateMetadata.BugSummary) != "" {
						jr.BugSummary = strings.TrimSpace(meta.GateMetadata.BugSummary)
					}
					if meta.GateMetadata != nil && meta.GateMetadata.StackGate != nil {
						if runtimeImage := strings.TrimSpace(meta.GateMetadata.StackGate.RuntimeImage); runtimeImage != "" {
							// Prefer the runtime-resolved gate image when available.
							jr.JobImage = runtimeImage
						}
					}
					if meta.RecoveryMetadata != nil {
						jr.Recovery = newRecoveryView(meta.RecoveryMetadata)
					} else if meta.GateMetadata != nil && meta.GateMetadata.Recovery != nil {
						jr.Recovery = newRecoveryView(meta.GateMetadata.Recovery)
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

			// Set timestamps.
			if job.StartedAt.Valid {
				t := job.StartedAt.Time.UTC()
				jr.StartedAt = &t
			}
			if job.FinishedAt.Valid {
				t := job.FinishedAt.Time.UTC()
				jr.FinishedAt = &t
			}

			resp.Jobs = append(resp.Jobs, jr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list run repo jobs: encode response failed", "err", err)
		}
	}
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

func newRecoveryView(meta *contracts.BuildGateRecoveryMetadata) *migsapi.RunRepoJobRecovery {
	if meta == nil {
		return nil
	}
	return &migsapi.RunRepoJobRecovery{
		LoopKind:                  meta.LoopKind,
		StrategyID:                meta.StrategyID,
		Confidence:                meta.Confidence,
		Reason:                    meta.Reason,
		Expectations:              meta.Expectations,
		CandidateSchemaID:         meta.CandidateSchemaID,
		CandidateArtifactPath:     meta.CandidateArtifactPath,
		CandidateValidationStatus: meta.CandidateValidationStatus,
		CandidateValidationError:  meta.CandidateValidationError,
		CandidatePromoted:         meta.CandidatePromoted,
	}
}
