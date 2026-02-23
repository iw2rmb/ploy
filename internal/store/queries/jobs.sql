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
  meta
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
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

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: ClaimJob :one
-- Atomically claim the next claimable job for a node (unified queue).
-- v1:
-- - claimable jobs have status='Queued'
-- - normal jobs are claimable only when runs.status='Started'
-- - MR jobs (job_type='mr') are claimable only when runs.status='Finished'
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
    AND (
      (j.job_type = 'mr' AND r.status = 'Finished') OR
      (j.job_type != 'mr' AND r.status = 'Started')
    )
  ORDER BY j.run_id ASC, j.repo_id ASC, j.attempt ASC, j.id ASC
  FOR UPDATE OF j SKIP LOCKED
  LIMIT 1
)
UPDATE jobs
SET status = 'Running', node_id = eligible.node_id, started_at = now()
FROM eligible
WHERE jobs.id = eligible.id
RETURNING jobs.*;

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
  jobs.meta;

-- name: CountJobsByRun :one
SELECT COUNT(*) FROM jobs
WHERE run_id = $1;

-- name: CountJobsByRunAndStatus :one
SELECT COUNT(*) FROM jobs
WHERE run_id = $1 AND status = $2;

-- name: UpdateJobCompletion :exec
UPDATE jobs
SET status = $2,
    exit_code = $3,
    finished_at = now(),
    duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT
WHERE id = $1;

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

-- name: UpdateJobCompletionWithMeta :exec
UPDATE jobs
SET status = $2,
    exit_code = $3,
    finished_at = now(),
    duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT,
    meta = $4
WHERE id = $1;

-- name: CountJobsByRunRepoAttemptGroupByStatus :many
-- Counts jobs by status for a specific repo attempt, excluding MR jobs.
-- Used by repo-scoped terminal detection to determine run_repos.status.
-- MR jobs (job_type='mr') are auxiliary and must not affect run_repos.status derivation.
SELECT status, COUNT(*)::int AS count
FROM jobs
WHERE run_id = $1
  AND repo_id = $2
  AND attempt = $3
  AND job_type != 'mr'
GROUP BY status;
