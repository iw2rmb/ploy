package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func (state *completeJobState) routedJobType() domaintypes.JobType {
	if state.serviceTypeOK {
		return state.jobType
	}
	return ""
}

func (s *CompleteJobService) onFail(ctx context.Context, state *completeJobState) {
	if state.input.Status != domaintypes.JobStatusFail && state.input.Status != domaintypes.JobStatusError {
		return
	}

	if state.input.Status == domaintypes.JobStatusFail {
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
	}
	if state.input.Status == domaintypes.JobStatusError {
		if errMsg := state.input.StatsPayload.ErrorMessage(); errMsg != "" {
			errText := errMsg
			if updateErr := s.store.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
				RunID:     state.job.RunID,
				RepoID:    state.job.RepoID,
				LastError: &errText,
			}); updateErr != nil {
				slog.Error("complete job: failed to set repo last_error from stats.error",
					"job_id", state.job.ID,
					"repo_id", state.job.RepoID,
					"err", updateErr,
				)
			}
		}
	}

	jobType := state.routedJobType()

	decision := lifecycle.EvaluateCompletionDecision(jobType, state.input.Status, state.job.NextID != nil)
	switch decision.ChainAction {
	case lifecycle.CompletionChainNoAction:
		return
	case lifecycle.CompletionChainEvaluateGateFailure:
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
	case lifecycle.CompletionChainCancelRemainder:
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
	jobType := state.routedJobType()
	decision := lifecycle.EvaluateCompletionDecision(jobType, state.input.Status, state.job.NextID != nil)
	if decision.ChainAction == lifecycle.CompletionChainNoAction {
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
	nextIDToPromote := state.job.NextID
	if state.serviceType == completeJobServiceTypeSBOM {
		sbomRowsPersisted, sbomErr := maybePersistSBOMRowsForJob(
			ctx,
			s.store,
			s.blobpersist,
			state.job.RunID,
			state.job.RepoID,
			state.job.ID,
		)
		if sbomErr != nil {
			slog.Error("complete job: persist sbom rows for completed sbom job failed",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"err", sbomErr,
			)
			return
		} else if sbomRowsPersisted > 0 {
			slog.Info("complete job: persisted sbom rows for completed sbom job",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"row_count", sbomRowsPersisted,
			)
		}
		hookNextID, hookPlanErr := s.planAndInsertCycleHookJobs(ctx, state)
		if hookPlanErr != nil {
			slog.Error("complete job: runtime hook planning for completed sbom job failed",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"err", hookPlanErr,
			)
			// Fail closed: do not advance to the original successor when hook planning
			// encountered an error, otherwise runnable hooks can be skipped.
			nextIDToPromote = nil
		} else if hookNextID != nil {
			nextIDToPromote = hookNextID
		}
	}

	jobType := state.routedJobType()
	if s.gateProfilesBS != nil {
		if state.serviceType == completeJobServiceTypeGate {
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
	if hookOnceErr := s.recordHookOnceLedger(ctx, state); hookOnceErr != nil {
		slog.Error("complete job: failed to record hook once state",
			"job_id", state.job.ID,
			"repo_id", state.job.RepoID,
			"err", hookOnceErr,
		)
	}

	decision := lifecycle.EvaluateCompletionDecision(jobType, state.input.Status, state.job.NextID != nil)
	if decision.ChainAction == lifecycle.CompletionChainAdvanceNext {
		if nextIDToPromote == nil {
			return
		}
		if _, err := s.store.PromoteJobByIDIfUnblocked(ctx, *nextIDToPromote); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("complete job: failed to promote next linked job",
				"job_id", state.job.ID,
				"next_id", nextIDToPromote,
				"err", err,
			)
		}
	}
}

func (s *CompleteJobService) reconcileRepoRun(ctx context.Context, state *completeJobState) {
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

	runRepo, runRepoErr := s.store.GetRunRepo(ctx, store.GetRunRepoParams{
		RunID:  state.job.RunID,
		RepoID: state.job.RepoID,
	})
	if runRepoErr != nil {
		slog.Error("complete job: failed to load run repo after status reconciliation",
			"job_id", state.job.ID,
			"repo_id", state.job.RepoID,
			"attempt", state.job.Attempt,
			"err", runRepoErr,
		)
	} else {
		if enqueueErr := enqueueAutoMRCreateAction(ctx, s.store, state.job.RunID, runRepo); enqueueErr != nil {
			slog.Error("complete job: failed to enqueue auto MR action",
				"job_id", state.job.ID,
				"run_id", state.job.RunID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"err", enqueueErr,
			)
		}
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
