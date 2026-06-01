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
		if errMsg := formatExit137Error(string(state.job.JobType), state.input.ExitCode); errMsg != nil {
			if updateErr := s.store.UpdateRunError(ctx, store.UpdateRunErrorParams{
				ID:        state.job.RunID,
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
			if updateErr := s.store.UpdateRunError(ctx, store.UpdateRunErrorParams{
				ID:        state.job.RunID,
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
	if state.input.Status == domaintypes.JobStatusFail && lifecycle.IsGateJobType(jobType) {
		if errMsg := formatStackGateError(jobType, state.persistedMeta); errMsg != nil {
			if updateErr := s.store.UpdateRunError(ctx, store.UpdateRunErrorParams{
				ID:        state.job.RunID,
				LastError: errMsg,
			}); updateErr != nil {
				slog.Error("complete job: failed to set repo last_error",
					"job_id", state.job.ID,
					"repo_id", state.job.RepoID,
					"err", updateErr,
				)
			}
		}
	}

	decision := lifecycle.EvaluateCompletionDecision(jobType, state.input.Status, state.job.NextID != nil)
	switch decision.ChainAction {
	case lifecycle.CompletionChainNoAction:
		return
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
	if lifecycle.IsGateJobType(state.jobType) {
		sbomRowsPersisted, sbomErr := maybePersistSBOMRowsForJob(
			ctx,
			s.store,
			s.blobpersist,
			state.job.RunID,
			state.job.RepoID,
			state.job.ID,
		)
		if sbomErr != nil {
			slog.Error("complete job: persist sbom rows for completed gate job failed",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"err", sbomErr,
			)
			return
		} else if sbomRowsPersisted > 0 {
			slog.Info("complete job: persisted sbom rows for completed gate job",
				"job_id", state.job.ID,
				"repo_id", state.job.RepoID,
				"attempt", state.job.Attempt,
				"row_count", sbomRowsPersisted,
			)
		}
	}

	jobType := state.routedJobType()

	decision := lifecycle.EvaluateCompletionDecision(jobType, state.input.Status, state.job.NextID != nil)
	if decision.ChainAction == lifecycle.CompletionChainAdvanceNext {
		if state.job.NextID == nil {
			return
		}
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
	repoUpdated, repoErr := recovery.MaybeUpdateRunStatus(ctx, s.store, state.job.RunID, state.job.Attempt)
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
