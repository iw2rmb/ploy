package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
)

func (s *CompleteJobService) onFail(ctx context.Context, state *completeJobState) {
	if state.input.Status != domaintypes.JobStatusFail {
		return
	}

	if errMsg := formatExit137Error(state.job.Name, state.input.ExitCode); errMsg != nil {
		if updateErr := s.store.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
			RunID:     state.job.RunID,
			RepoID:    state.job.RepoID,
			LastError: errMsg,
		}); updateErr != nil {
			slog.Error("complete job: failed to set repo last_error for exit code 137",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"err", updateErr,
			)
		}
	}

	jobType := domaintypes.JobType(state.job.JobType)
	if err := jobType.Validate(); err != nil {
		slog.Error("complete job: invalid job_type in job record; treating as non-gate for failure handling",
			"job_id", state.job.ID,
			"job_type", state.job.JobType,
			"err", err,
		)
		jobType = ""
	}

	switch jobType {
	case domaintypes.JobTypeMR:
		slog.Warn("complete job: MR job failed; ignoring for run-level failure handling",
			"job_id", state.job.ID,
			"next_id", state.job.NextID,
		)
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		if errMsg := formatStackGateError(jobType, state.persistedMeta); errMsg != nil {
			if updateErr := s.store.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
				RunID:     state.job.RunID,
				RepoID:    state.job.RepoID,
				LastError: errMsg,
			}); updateErr != nil {
				slog.Error("complete job: failed to set repo last_error",
					"job_id", state.job.ID,
					"repo_id", state.job.RepoID,
					"err", updateErr,
				)
			}
		}
		run, ok := s.loadRunForPostCompletion(ctx, state, "healing insertion")
		if ok {
			if healErr := maybeCreateHealingJobs(ctx, s.store, s.blobpersist, run, state.job); healErr != nil {
				slog.Error("complete job: failed to create healing jobs",
					"job_id", state.job.ID,
					"next_id", state.job.NextID,
					"err", healErr,
				)
			}
		}
	default:
		if err := cancelRemainingJobsAfterFailure(ctx, s.store, state.job); err != nil {
			slog.Error("complete job: failed to cancel remaining jobs after non-gate failure",
				"job_id", state.job.ID,
				"next_id", state.job.NextID,
				"err", err,
			)
		}
	}
}

func (s *CompleteJobService) onCancelled(ctx context.Context, state *completeJobState) {
	if state.input.Status != domaintypes.JobStatusCancelled {
		return
	}
	jobType := domaintypes.JobType(state.job.JobType)
	if jobType.Validate() == nil && jobType == domaintypes.JobTypeMR {
		return
	}
	if err := cancelRemainingJobsAfterFailure(ctx, s.store, state.job); err != nil {
		slog.Error("complete job: failed to cancel remaining jobs after cancellation",
			"job_id", state.job.ID,
			"next_id", state.job.NextID,
			"err", err,
		)
	}
}

func (s *CompleteJobService) onSuccess(ctx context.Context, state *completeJobState) {
	if state.input.Status != domaintypes.JobStatusSuccess {
		return
	}

	jobType := domaintypes.JobType(state.job.JobType)
	if s.gateProfilesBS != nil {
		if jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate {
			run, ok := s.loadRunForPostCompletion(ctx, state, "gate profile persistence")
			if ok {
				specRow, specErr := s.store.GetSpec(ctx, run.SpecID)
				if specErr != nil {
					slog.Error("complete job: failed to load spec for gate profile persistence",
						"job_id", state.job.ID,
						"run_id", run.ID,
						"spec_id", run.SpecID,
						"err", specErr,
					)
				} else if persistErr := persistSuccessfulGateProfile(ctx, s.store, s.gateProfilesBS, state.job, state.persistedMeta, specRow.Spec); persistErr != nil {
					slog.Error("complete job: failed to persist successful gate profile",
						"job_id", state.job.ID,
						"repo_id", state.job.RepoID,
						"err", persistErr,
					)
				}
			}
		}
	}

	if promoteErr := maybePromoteReGateRecoveryCandidate(ctx, s.store, s.gateProfilesBS, state.job, state.persistedMeta); promoteErr != nil {
		slog.Error("complete job: failed to promote validated re-gate candidate",
			"job_id", state.job.ID,
			"repo_id", state.job.RepoID,
			"err", promoteErr,
		)
	}
	if refreshErr := maybeRefreshNextReGateRecoveryCandidate(ctx, s.store, s.blobpersist, state.job); refreshErr != nil {
		slog.Error("complete job: failed to refresh next re-gate recovery candidate",
			"job_id", state.job.ID,
			"repo_id", state.job.RepoID,
			"err", refreshErr,
		)
	}

	if state.job.NextID != nil {
		if _, err := s.store.PromoteJobByIDIfUnblocked(ctx, *state.job.NextID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("complete job: failed to promote next linked job",
				"job_id", state.job.ID,
				"next_id", state.job.NextID,
				"err", err,
			)
		}
	}
}

func (s *CompleteJobService) reconcileRepoRun(ctx context.Context, state *completeJobState) {
	jobType := domaintypes.JobType(state.job.JobType)
	isMRJob := jobType.Validate() == nil && jobType == domaintypes.JobTypeMR
	if isMRJob {
		return
	}

	repoUpdated, repoErr := recovery.MaybeUpdateRunRepoStatus(ctx, s.store, state.job.RunID, state.job.RepoID, state.job.Attempt)
	if repoErr != nil {
		slog.Error("complete job: failed to check repo completion",
			"job_id", state.job.ID,
			"repo_id", state.job.RepoID,
			"attempt", state.job.Attempt,
			"err", repoErr,
		)
		return
	}
	if !repoUpdated {
		return
	}

	run, ok := s.loadRunForPostCompletion(ctx, state, "run completion reconciliation")
	if !ok {
		return
	}
	if _, completeErr := recovery.MaybeCompleteRunIfAllReposTerminal(ctx, s.store, s.eventsService, run); completeErr != nil {
		slog.Error("complete job: failed to check run completion",
			"job_id", state.job.ID,
			"next_id", state.job.NextID,
			"err", completeErr,
		)
	}
}

func (s *CompleteJobService) mergeMRURL(ctx context.Context, state *completeJobState) {
	jobType := domaintypes.JobType(state.job.JobType)
	if jobType.Validate() != nil || jobType != domaintypes.JobTypeMR {
		return
	}
	mrURL := state.input.StatsPayload.MRURL()
	if mrURL == "" {
		return
	}
	if updateErr := s.store.UpdateRunStatsMRURL(ctx, store.UpdateRunStatsMRURLParams{
		ID:    state.job.RunID,
		MrUrl: mrURL,
	}); updateErr != nil {
		slog.Error("complete job: failed to merge MR URL into run stats",
			"job_id", state.job.ID,
			"run_id", state.job.RunID,
			"err", updateErr,
		)
	}
}
