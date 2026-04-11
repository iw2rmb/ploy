-- name: GetJob :one
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  cache_key,
  meta
FROM jobs
WHERE id = $1;

-- name: ListJobsByRun :many
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  cache_key,
  meta
FROM jobs
WHERE run_id = $1
ORDER BY repo_id ASC, attempt ASC, id ASC;

-- name: ListJobsByRunRepoAttempt :many
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  cache_key,
  meta
FROM jobs
WHERE run_id = $1 AND repo_id = $2 AND attempt = $3
ORDER BY id ASC;

-- name: CreateJob :one
-- Note: `id` is a required TEXT parameter (KSUID-backed); caller generates via types.NewJobID().
INSERT INTO jobs (
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  meta,
  repo_sha_in,
  repo_sha_in8
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
  CASE WHEN $12::TEXT = '' THEN '' ELSE SUBSTRING($12::TEXT, 1, 8) END
)
RETURNING
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  cache_key,
  meta;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2,
    -- started_at: set when transitioning to Running (defensive; preserves existing started_at).
    started_at = CASE
      WHEN $2 = 'Running'::job_status AND started_at IS NULL THEN now()
      WHEN $2 = 'Running'::job_status THEN started_at
      ELSE $3
    END,
    finished_at = $4,
    duration_ms = $5
WHERE id = $1;

-- name: CancelActiveJobsByRun :execrows
-- Bulk-cancels active jobs for a run (Created/Queued/Running -> Cancelled).
-- finished_at is set once; duration_ms is computed from started_at when present.
UPDATE jobs
SET status = 'Cancelled',
    finished_at = COALESCE(finished_at, now()),
    duration_ms = CASE
      WHEN started_at IS NULL THEN 0
      ELSE GREATEST(EXTRACT(EPOCH FROM (COALESCE(finished_at, now()) - started_at)) * 1000, 0)::BIGINT
    END
WHERE run_id = $1
  AND status IN ('Created', 'Queued', 'Running');

-- name: CancelActiveJobsByRunRepoAttempt :execrows
-- Bulk-cancels active jobs for a specific repo attempt.
-- Targets Created/Queued/Running and preserves terminal jobs.
-- finished_at is set once; duration_ms is computed from started_at when present.
UPDATE jobs
SET status = 'Cancelled',
    finished_at = COALESCE(finished_at, now()),
    duration_ms = CASE
      WHEN started_at IS NULL THEN 0
      ELSE GREATEST(EXTRACT(EPOCH FROM (COALESCE(finished_at, now()) - started_at)) * 1000, 0)::BIGINT
    END
WHERE run_id = $1
  AND repo_id = $2
  AND attempt = $3
  AND status IN ('Created', 'Queued', 'Running');

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: ClaimJob :one
-- Atomically claim the next claimable job for a node (unified queue).
-- v1:
-- - claimable jobs have status='Queued'
-- - jobs are claimable only when runs.status='Started'
-- - nodeID must be non-empty
WITH eligible AS (
  SELECT j.id, n.id AS node_id
  FROM nodes n
  JOIN jobs j ON TRUE
  JOIN runs r ON j.run_id = r.id
  WHERE n.id = @node_id
    AND @node_id::TEXT != ''
    AND j.status = 'Queued'
    AND j.node_id IS NULL
    AND r.status = 'Started'
  ORDER BY j.run_id ASC, j.repo_id ASC, j.attempt ASC, j.id ASC
  FOR UPDATE OF j SKIP LOCKED
  LIMIT 1
)
UPDATE jobs
SET status = 'Running', node_id = eligible.node_id, started_at = now()
FROM eligible
WHERE jobs.id = eligible.id
RETURNING jobs.*;

-- name: ListStaleRunningJobs :many
-- Lists running jobs whose assigned node is stale at the provided cutoff.
-- Rows are grouped by (run_id, repo_id, attempt) for deterministic recovery processing.
SELECT
  jobs.run_id,
  jobs.repo_id,
  jobs.attempt,
  COUNT(*)::int AS running_jobs
FROM jobs
LEFT JOIN nodes ON nodes.id = jobs.node_id
WHERE jobs.status = 'Running'
  AND (
    jobs.node_id IS NULL
    OR nodes.last_heartbeat IS NULL
    OR nodes.last_heartbeat < $1
  )
GROUP BY jobs.run_id, jobs.repo_id, jobs.attempt
ORDER BY jobs.run_id ASC, jobs.repo_id ASC, jobs.attempt ASC;

-- name: CountStaleNodesWithRunningJobs :one
-- Counts distinct stale nodes that currently have at least one running job.
-- Excludes NULL node_id rows (orphaned running jobs) from node count.
SELECT COUNT(DISTINCT jobs.node_id)::BIGINT
FROM jobs
JOIN nodes ON nodes.id = jobs.node_id
WHERE jobs.status = 'Running'
  AND (
    nodes.last_heartbeat IS NULL
    OR nodes.last_heartbeat < $1
  );

-- name: GetAdjacentJobIndices :one
-- Transitional: returns current job id and linked successor id.
SELECT
  j1.id AS prev_id,
  j1.next_id AS next_id
FROM jobs j1
WHERE j1.id = $1;

-- name: ListCreatedJobsByRunRepoAttempt :many
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  name,
  status,
  job_type,
  job_image,
  next_id,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  cache_key,
  meta
FROM jobs
WHERE run_id = $1 AND repo_id = $2 AND attempt = $3 AND status = 'Created'
ORDER BY id ASC;

-- name: ScheduleNextJob :one
-- Atomically promote the next unblocked job in a repo attempt: Created -> Queued.
-- A created job is unblocked when all predecessor jobs that point to it are Success.
WITH next_job AS (
  SELECT j.id
  FROM jobs j
  WHERE j.run_id = $1
    AND j.repo_id = $2
    AND j.attempt = $3
    AND j.status = 'Created'
    AND NOT EXISTS (
      SELECT 1
      FROM jobs p
      WHERE p.run_id = j.run_id
        AND p.repo_id = j.repo_id
        AND p.attempt = j.attempt
        AND p.next_id = j.id
        AND p.status != 'Success'
    )
  ORDER BY j.id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE jobs
SET status = 'Queued'
FROM next_job
WHERE jobs.id = next_job.id
  AND jobs.status = 'Created'
RETURNING
  jobs.id,
  jobs.run_id,
  jobs.repo_id,
  jobs.repo_base_ref,
  jobs.attempt,
  jobs.name,
  jobs.status,
  jobs.job_type,
  jobs.job_image,
  jobs.next_id,
  jobs.node_id,
  jobs.exit_code,
  jobs.started_at,
  jobs.finished_at,
  jobs.duration_ms,
  jobs.repo_sha_in,
  jobs.repo_sha_out,
  jobs.repo_sha_in8,
  jobs.repo_sha_out8,
  jobs.cache_key,
  jobs.meta;

-- name: PromoteJobByIDIfUnblocked :one
-- Atomically promote a specific linked successor job: Created -> Queued.
-- The candidate is eligible only when every predecessor that points to it is Success.
WITH candidate AS (
  SELECT j.id
  FROM jobs j
  WHERE j.id = $1
    AND j.status = 'Created'
    AND NOT EXISTS (
      SELECT 1
      FROM jobs p
      WHERE p.next_id = j.id
        AND p.status != 'Success'
    )
  FOR UPDATE SKIP LOCKED
)
UPDATE jobs
SET status = 'Queued'
FROM candidate
WHERE jobs.id = candidate.id
  AND jobs.status = 'Created'
RETURNING
  jobs.id,
  jobs.run_id,
  jobs.repo_id,
  jobs.repo_base_ref,
  jobs.attempt,
  jobs.name,
  jobs.status,
  jobs.job_type,
  jobs.job_image,
  jobs.next_id,
  jobs.node_id,
  jobs.exit_code,
  jobs.started_at,
  jobs.finished_at,
  jobs.duration_ms,
  jobs.repo_sha_in,
  jobs.repo_sha_out,
  jobs.repo_sha_in8,
  jobs.repo_sha_out8,
  jobs.cache_key,
  jobs.meta;

-- name: UpdateJobNextID :exec
UPDATE jobs
SET next_id = $2
WHERE id = $1;

-- name: UpdateJobRepoSHAIn :exec
UPDATE jobs
SET repo_sha_in = $2,
    repo_sha_in8 = CASE
      WHEN $2::TEXT = '' THEN ''
      ELSE SUBSTRING($2::TEXT, 1, 8)
    END
WHERE id = $1;

-- name: ClearRepoSHAChainFromJob :execrows
WITH RECURSIVE chain AS (
  SELECT j.id, j.next_id
  FROM jobs j
  WHERE j.id = $1
    AND j.run_id = $2
    AND j.repo_id = $3
    AND j.attempt = $4
    AND j.status IN ('Created', 'Queued')
  UNION ALL
  SELECT n.id, n.next_id
  FROM jobs n
  JOIN chain c ON n.id = c.next_id
  WHERE n.run_id = $2
    AND n.repo_id = $3
    AND n.attempt = $4
    AND n.status IN ('Created', 'Queued')
)
UPDATE jobs AS j
SET repo_sha_in = '',
    repo_sha_out = '',
    repo_sha_in8 = '',
    repo_sha_out8 = ''
FROM chain
WHERE j.id = chain.id;

-- name: CountJobsByRun :one
SELECT COUNT(*) FROM jobs
WHERE run_id = $1;

-- name: CountJobsByRunAndStatus :one
SELECT COUNT(*) FROM jobs
WHERE run_id = $1 AND status = $2;

-- name: UpdateJobCompletion :exec
WITH completed AS (
  UPDATE jobs
  SET status = sqlc.arg(status),
      exit_code = sqlc.arg(exit_code),
      repo_sha_out = CASE
        WHEN sqlc.arg(repo_sha_out)::TEXT = '' THEN repo_sha_out
        ELSE sqlc.arg(repo_sha_out)::TEXT
      END,
      repo_sha_out8 = CASE
        WHEN sqlc.arg(repo_sha_out)::TEXT = '' THEN repo_sha_out8
        ELSE SUBSTRING(sqlc.arg(repo_sha_out)::TEXT, 1, 8)
      END,
      finished_at = now(),
      duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT
  WHERE jobs.id = sqlc.arg(id)
  RETURNING next_id, repo_sha_out
)
UPDATE jobs AS next_job
SET repo_sha_in = CASE
      WHEN completed.repo_sha_out = '' THEN next_job.repo_sha_in
      ELSE completed.repo_sha_out
    END,
    repo_sha_in8 = CASE
      WHEN completed.repo_sha_out = '' THEN next_job.repo_sha_in8
      ELSE SUBSTRING(completed.repo_sha_out, 1, 8)
    END
FROM completed
WHERE next_job.id = completed.next_id;

-- name: UpdateJobMeta :exec
UPDATE jobs
SET meta = $2
WHERE id = $1;

-- name: UpdateJobImageName :exec
-- Persist the container image name used to execute a job.
-- This is set by the node immediately before job execution starts.
UPDATE jobs
SET job_image = $2
WHERE id = $1;

-- name: UpdateJobCacheKey :exec
UPDATE jobs
SET cache_key = $2
WHERE id = $1;

-- name: ResolveReusableJobByCacheKey :one
SELECT
  id,
  status,
  exit_code,
  repo_sha_out,
  meta
FROM jobs
WHERE repo_id = sqlc.arg(repo_id)
  AND job_type = sqlc.arg(job_type)
  AND cache_key = sqlc.arg(cache_key)
  AND cache_key <> ''
  AND status IN ('Success', 'Fail')
  AND (
    status <> 'Fail'
    OR EXISTS (
      SELECT 1
      FROM logs
      WHERE logs.run_id = jobs.run_id
        AND logs.job_id = jobs.id
    )
  )
  AND NOT (meta ? 'cache_mirror')
ORDER BY finished_at DESC NULLS LAST, id DESC
LIMIT 1;

-- name: UpdateJobCompletionWithMeta :exec
WITH completed AS (
  UPDATE jobs
  SET status = sqlc.arg(status),
      exit_code = sqlc.arg(exit_code),
      repo_sha_out = CASE
        WHEN sqlc.arg(repo_sha_out)::TEXT = '' THEN repo_sha_out
        ELSE sqlc.arg(repo_sha_out)::TEXT
      END,
      repo_sha_out8 = CASE
        WHEN sqlc.arg(repo_sha_out)::TEXT = '' THEN repo_sha_out8
        ELSE SUBSTRING(sqlc.arg(repo_sha_out)::TEXT, 1, 8)
      END,
      finished_at = now(),
      duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT,
      meta = sqlc.arg(meta)
  WHERE jobs.id = sqlc.arg(id)
  RETURNING next_id, repo_sha_out
)
UPDATE jobs AS next_job
SET repo_sha_in = CASE
      WHEN completed.repo_sha_out = '' THEN next_job.repo_sha_in
      ELSE completed.repo_sha_out
    END,
    repo_sha_in8 = CASE
      WHEN completed.repo_sha_out = '' THEN next_job.repo_sha_in8
      ELSE SUBSTRING(completed.repo_sha_out, 1, 8)
    END
FROM completed
WHERE next_job.id = completed.next_id;

-- name: CountJobsByRunRepoAttemptGroupByStatus :many
-- Counts jobs by status for a specific repo attempt.
-- Used by repo-scoped terminal detection to determine run_repos.status.
SELECT status, COUNT(*)::int AS count
FROM jobs
WHERE run_id = $1
  AND repo_id = $2
  AND attempt = $3
GROUP BY status;

-- name: ListJobsForTUI :many
-- Lists jobs with optional run_id filter, ordered newest-to-oldest by job id.
-- run_id: if non-null, filter to jobs for that run; if null, return all jobs.
-- Joins runs and migs to surface mig_name per job for the TUI jobs-list screen.
SELECT
  jobs.id AS job_id,
  jobs.name,
  jobs.job_type,
  jobs.status,
  jobs.duration_ms,
  jobs.job_image,
  jobs.node_id,
  migs.name AS mig_name,
  jobs.run_id,
  jobs.repo_id
FROM jobs
JOIN runs ON jobs.run_id = runs.id
JOIN migs ON runs.mig_id = migs.id
WHERE (sqlc.narg(run_id)::text IS NULL OR jobs.run_id = sqlc.narg(run_id)::text)
ORDER BY jobs.id DESC
LIMIT $1 OFFSET $2;

-- name: CountJobsForTUI :one
-- Counts jobs with optional run_id filter.
-- run_id: if non-null, count jobs for that run; if null, count all jobs.
-- Used with ListJobsForTUI to provide total for TUI pagination.
SELECT COUNT(jobs.id)::BIGINT
FROM jobs
JOIN runs ON jobs.run_id = runs.id
JOIN migs ON runs.mig_id = migs.id
WHERE (sqlc.narg(run_id)::text IS NULL OR jobs.run_id = sqlc.narg(run_id)::text);
