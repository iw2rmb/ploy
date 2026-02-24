package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// maybeUpdateRunRepoStatus derives and persists run_repos.status from job outcomes.
// Called after a job completes to check if the repo attempt has reached a terminal state.
//
// Repo-scoped status computation:
// - On job terminal for the last step in a repo: compute and persist run_repos.status.
// - MR jobs (job_type='mr') are excluded from terminal computation.
//
// Terminal status derivation rules:
// - Cancelled: if the last job is Cancelled
// - Otherwise: equal to the status of the last job
//
// Returns true if the repo status was updated to terminal, false otherwise.
func maybeUpdateRunRepoStatus(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.ModRepoID,
	attempt int32,
) (bool, error) {
	// List jobs for this repo attempt and compute terminal status from the last job
	// (highest next_id), excluding MR jobs.
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return false, fmt.Errorf("list jobs by repo attempt: %w", err)
	}

	var lastJob *store.Job
	for i := range jobs {
		job := &jobs[i]

		mt := domaintypes.JobType(job.JobType)
		if mt.Validate() == nil && mt == domaintypes.JobTypeMR {
			continue
		}

		// If any non-MR job is non-terminal, the repo attempt is still in progress.
		switch job.Status {
		case store.JobStatusSuccess, store.JobStatusFail, store.JobStatusCancelled:
			// terminal
		default:
			return false, nil
		}

		currentStep := jobStepIndex(*job)
		if !currentStep.Valid() {
			return false, fmt.Errorf("invalid next_id for job_id=%s next_id=%v", job.ID, float64(currentStep))
		}

		if lastJob == nil || currentStep.Float64() > jobStepIndex(*lastJob).Float64() {
			lastJob = job
		}
	}

	if lastJob == nil {
		return false, nil
	}

	var repoStatus store.RunRepoStatus
	switch lastJob.Status {
	case store.JobStatusSuccess:
		repoStatus = store.RunRepoStatusSuccess
	case store.JobStatusFail:
		repoStatus = store.RunRepoStatusFail
	case store.JobStatusCancelled:
		repoStatus = store.RunRepoStatusCancelled
	default:
		// Should be unreachable due to terminal guard above.
		return false, fmt.Errorf("unexpected last job status %q for job_id=%s", lastJob.Status, lastJob.ID)
	}

	// Update run_repos.status and finished_at timestamp.
	if err := st.UpdateRunRepoStatus(ctx, store.UpdateRunRepoStatusParams{
		RunID:  runID,
		RepoID: repoID,
		Status: repoStatus,
	}); err != nil {
		return false, fmt.Errorf("update run repo status: %w", err)
	}

	slog.Info("run repo completed",
		"run_id", runID,
		"repo_id", repoID,
		"attempt", attempt,
		"status", repoStatus,
	)

	return true, nil
}

// maybeCompleteRunIfAllReposTerminal transitions runs.status to Finished only when
// all run_repos are terminal (Success/Fail/Cancelled), and publishes the run and
// done SSE events.
func maybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID domaintypes.RunID) error {
	if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
		return nil
	}

	counts, err := st.CountRunReposByStatus(ctx, runID)
	if err != nil {
		return fmt.Errorf("count run repos: %w", err)
	}

	var (
		total        int32
		terminal     int32
		anyFail      bool
		anyCancelled bool
	)
	for _, row := range counts {
		total += row.Count
		switch row.Status {
		case store.RunRepoStatusSuccess, store.RunRepoStatusFail, store.RunRepoStatusCancelled:
			terminal += row.Count
		}
		if row.Status == store.RunRepoStatusFail && row.Count > 0 {
			anyFail = true
		}
		if row.Status == store.RunRepoStatusCancelled && row.Count > 0 {
			anyCancelled = true
		}
	}

	if total == 0 || terminal < total {
		return nil
	}

	if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: runID, Status: store.RunStatusFinished}); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	if eventsService != nil {
		repoURL := ""
		if repos, err := st.ListRunReposByRun(ctx, runID); err == nil && len(repos) > 0 {
			if mr, err := st.GetModRepo(ctx, repos[0].RepoID); err == nil {
				repoURL = mr.RepoUrl
			}
		}

		runState := modsapi.RunStateSucceeded
		if anyFail {
			runState = modsapi.RunStateFailed
		} else if anyCancelled {
			runState = modsapi.RunStateCancelled
		}

		summary := modsapi.RunSummary{
			RunID:      runID,
			State:      runState,
			Repository: repoURL,
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[domaintypes.JobID]modsapi.StageStatus),
		}
		if err := eventsService.PublishRun(ctx, runID, summary); err != nil {
			slog.Error("complete run: publish run event failed", "run_id", runID, "err", err)
		}
		if err := eventsService.Hub().PublishStatus(ctx, runID, logstream.Status{Status: "done"}); err != nil {
			slog.Error("complete run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("run completed", "run_id", runID, "status", store.RunStatusFinished)
	return nil
}
