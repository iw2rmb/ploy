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

	var (
		tailJobs         []store.Job
		fallback         *store.Job
		lastByNextIDMeta *store.Job
		maxNextIDMeta    float64
		hasNextIDMeta    bool
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

		if fallback == nil || job.ID.String() > fallback.ID.String() {
			fallback = job
		}
		if nextID, ok := nextIDFromMeta(job.Meta); ok {
			if !hasNextIDMeta || nextID > maxNextIDMeta || (nextID == maxNextIDMeta && (lastByNextIDMeta == nil || job.ID.String() > lastByNextIDMeta.ID.String())) {
				maxNextIDMeta = nextID
				lastByNextIDMeta = job
				hasNextIDMeta = true
			}
		}
		if job.NextID == nil || job.NextID.IsZero() {
			tailJobs = append(tailJobs, *job)
		}
	}

	if fallback == nil {
		return false, nil
	}
	lastJob := fallback
	if hasNextIDMeta && lastByNextIDMeta != nil {
		lastJob = lastByNextIDMeta
	}
	if len(tailJobs) > 0 {
		lastJob = &tailJobs[0]
		for i := 1; i < len(tailJobs); i++ {
			if tailJobs[i].ID.String() > lastJob.ID.String() {
				lastJob = &tailJobs[i]
			}
		}
		if hasNextIDMeta && lastByNextIDMeta != nil {
			lastJob = lastByNextIDMeta
		}
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

func nextIDFromMeta(meta []byte) (float64, bool) {
	if len(meta) == 0 {
		return 0, false
	}
	var raw map[string]any
	if err := json.Unmarshal(meta, &raw); err != nil {
		return 0, false
	}
	v, ok := raw["next_id"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// MaybeCompleteRunIfAllReposTerminal transitions runs.status to Finished only when
// all run_repos are terminal (Success/Fail/Cancelled), and publishes the run and
// done SSE events.
func MaybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *server.EventsService, run store.Run, runID domaintypes.RunID) error {
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
			if mr, err := st.GetMigRepo(ctx, repos[0].RepoID); err == nil {
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

func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
