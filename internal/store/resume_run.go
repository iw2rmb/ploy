package store

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
)

func resumeFailedJobs(ctx context.Context, q *Queries, run Run, arg RestartRunParams) error {
	if (run.Status != types.RunStatusFail && run.Status != types.RunStatusCancelled) || types.RunStats(run.Stats).ResumeCount() != arg.ExpectedResumeCount {
		return ErrRunResumeInvalid
	}
	// Completion takes the same run lock, so the chain cannot change while it is reset.
	jobs, err := q.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: run.ID, Attempt: run.Attempt})
	if err != nil {
		return err
	}
	suffix, err := failedJobSuffix(jobs)
	if err != nil {
		return err
	}
	plans := make(map[string]JobPlan, len(arg.Jobs))
	for _, plan := range arg.Jobs {
		plans[plan.Name] = plan
	}
	for i, job := range suffix {
		plan, ok := plans[job.Name]
		if !ok || plan.JobType != job.JobType {
			return fmt.Errorf("%w: step configuration missing for %s", ErrRunResumeInvalid, job.ID)
		}
		status := types.JobStatusCreated
		if i == 0 {
			status = types.JobStatusQueued
		}
		// Keep the failed job's node as the affinity anchor, including a failed chain head.
		// Only its input SHA survives; successor inputs are supplied by normal completion.
		_, err = q.db.Exec(ctx, `UPDATE jobs SET status=$2, exit_code=NULL, started_at=NULL,
   finished_at=NULL, duration_ms=0, repo_sha_out='', repo_sha_out8='', meta=$3,
   repo_sha_in=CASE WHEN $4 THEN repo_sha_in ELSE '' END,
   repo_sha_in8=CASE WHEN $4 THEN repo_sha_in8 ELSE '' END
   WHERE id=$1`, job.ID, status, plan.Meta, i == 0)
		if err != nil {
			return err
		}
		if _, err = q.db.Exec(ctx, `DELETE FROM logs WHERE job_id=$1`, job.ID); err != nil {
			return err
		}
		if err = q.DeleteSBOMRowsByJob(ctx, job.ID); err != nil {
			return err
		}
		if _, err = q.db.Exec(ctx, `DELETE FROM job_metrics WHERE job_id=$1`, job.ID); err != nil {
			return err
		}
	}
	stats := arg.Stats
	if len(stats) == 0 {
		stats = []byte(`{}`)
	}
	_, err = q.db.Exec(ctx, `UPDATE runs SET status='Running', finished_at=NULL, last_error=NULL,
  stats=stats || $2::jsonb || jsonb_build_object('resume_count', $3::int, 'last_resumed_at', now())
  WHERE id=$1`, run.ID, stats, arg.ExpectedResumeCount+1)
	return err
}

// The linked chain is authoritative; IDs and query order do not encode step order.
func failedJobSuffix(jobs []Job) ([]Job, error) {
	chain := jobchain.Order(jobs, func(j Job) types.JobID { return j.ID }, func(j Job) *types.JobID { return j.NextID })
	// Order also supports display of malformed chains; resuming requires one complete chain.
	if len(chain) == 0 || len(chain) != len(jobs) {
		return nil, ErrRunResumeInvalid
	}
	for i, job := range chain {
		if i == len(chain)-1 {
			if job.NextID != nil {
				return nil, ErrRunResumeInvalid
			}
		} else if job.NextID == nil || *job.NextID != chain[i+1].ID {
			return nil, ErrRunResumeInvalid
		}
	}
	failed := -1
	var owner types.NodeID
	for i, job := range chain {
		if job.NodeID != nil {
			if owner != "" && owner != *job.NodeID {
				return nil, ErrRunResumeInvalid
			}
			owner = *job.NodeID
		}
		if failed < 0 && job.Status == types.JobStatusSuccess {
			continue
		}
		if failed < 0 {
			if (job.Status != types.JobStatusFail && job.Status != types.JobStatusError) || job.NodeID == nil || *job.NodeID == "" || !types.IsCanonicalFullCommitSHA(job.RepoShaIn) {
				return nil, ErrRunResumeInvalid
			}
			failed = i
			continue
		}
		if job.Status != types.JobStatusCancelled || job.StartedAt.Valid || job.NodeID != nil {
			return nil, ErrRunResumeInvalid
		}
	}
	if failed < 0 {
		return nil, ErrRunResumeInvalid
	}
	return chain[failed:], nil
}

// WithJobExecution fences completion against restart for the entire database
// transition, including successor promotion and run reconciliation. A delayed
// request must not complete a reused job ID after a retained-workspace retry.
func (s *PgStore) WithJobExecution(ctx context.Context, jobID types.JobID, resumeCount int, complete func(Store, Job) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT id FROM runs WHERE id=$1 FOR NO KEY UPDATE`, job.RunID)
	if err != nil {
		return err
	}
	run, err := q.GetRun(ctx, job.RunID)
	if err != nil {
		return err
	}
	if types.RunStats(run.Stats).ResumeCount() != resumeCount || job.Attempt != run.Attempt {
		return ErrJobExecutionStale
	}
	job, err = q.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	scoped := &PgStore{pool: s.pool, Queries: q}
	if err = complete(scoped, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
