package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaybeUpdateRunRepoStatus derives and persists run_repos.status from job outcomes.
// Called after state changes to check if the repo attempt has reached a terminal state.
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
func MaybeUpdateRunRepoStatus(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.MigRepoID,
	attempt int32,
) (bool, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return false, fmt.Errorf("list jobs by repo attempt: %w", err)
	}

	// Collect candidates for "last job" with priority:
	//   1. Highest next_id from job metadata (explicit ordering)
	//   2. Tail job (NextID is nil/zero — end of chain), latest by ID
	//   3. Fallback: latest job by ID
	var (
		byNextIDMeta  *store.Job
		maxNextIDMeta int64
		bestTail      *store.Job
		bestFallback  *store.Job
	)
	for i := range jobs {
		job := &jobs[i]

		mt := domaintypes.JobType(job.JobType)
		if mt.Validate() == nil && mt == domaintypes.JobTypeMR {
			continue
		}

		switch job.Status {
		case store.JobStatusSuccess, store.JobStatusFail, store.JobStatusCancelled:
			// terminal
		default:
			return false, nil
		}

		if bestFallback == nil || job.ID.String() > bestFallback.ID.String() {
			bestFallback = job
		}
		if nextID, ok := nextIDFromMeta(job.Meta); ok {
			if byNextIDMeta == nil || nextID > maxNextIDMeta || (nextID == maxNextIDMeta && job.ID.String() > byNextIDMeta.ID.String()) {
				maxNextIDMeta = nextID
				byNextIDMeta = job
			}
		}
		if job.NextID == nil || job.NextID.IsZero() {
			if bestTail == nil || job.ID.String() > bestTail.ID.String() {
				bestTail = job
			}
		}
	}

	if bestFallback == nil {
		return false, nil
	}

	// Select last job by priority.
	lastJob := bestFallback
	if bestTail != nil {
		lastJob = bestTail
	}
	if byNextIDMeta != nil {
		lastJob = byNextIDMeta
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
		return false, fmt.Errorf("unexpected last job status %q for job_id=%s", lastJob.Status, lastJob.ID)
	}

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

func nextIDFromMeta(meta []byte) (int64, bool) {
	if len(meta) == 0 {
		return 0, false
	}
	var m struct {
		NextID *int64 `json:"next_id"`
	}
	if err := json.Unmarshal(meta, &m); err != nil || m.NextID == nil {
		return 0, false
	}
	return *m.NextID, true
}

// MaybeCompleteRunIfAllReposTerminal transitions runs.status to Finished only when
// all run_repos are terminal (Success/Fail/Cancelled), and publishes the run and
// done SSE events.
//
// Returns true when the run transitions to terminal in this call.
func MaybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *server.EventsService, run store.Run) (bool, error) {
	runID := run.ID
	if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
		return false, nil
	}

	counts, err := st.CountRunReposByStatus(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("count run repos: %w", err)
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
		return false, nil
	}

	// Re-read current run status before finalization so concurrent or repeated
	// reconciliation paths do not emit duplicate terminal transitions/events.
	currentRun, err := st.GetRun(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("get run for completion check: %w", err)
	}
	if currentRun.Status == store.RunStatusFinished || currentRun.Status == store.RunStatusCancelled {
		return false, nil
	}

	if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: runID, Status: store.RunStatusFinished}); err != nil {
		return false, fmt.Errorf("update run status: %w", err)
	}

	if eventsService != nil {
		repoURL := ""
		if repos, err := st.ListRunReposWithURLByRun(ctx, runID); err == nil && len(repos) > 0 {
			repoURL = repos[0].RepoUrl
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
	return true, nil
}

func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
