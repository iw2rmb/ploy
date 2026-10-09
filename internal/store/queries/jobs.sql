-- name: GetJob :one
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  status,
  job_type,
  job_image,
  next_id,
  name,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
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
  status,
  job_type,
  job_image,
  next_id,
  name,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  meta
FROM jobs
WHERE run_id = $1
ORDER BY attempt ASC, id ASC;

-- name: ListJobsByRunAttempt :many
SELECT
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  status,
  job_type,
  job_image,
  next_id,
  name,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
  meta
FROM jobs
WHERE run_id = $1 AND attempt = $2
ORDER BY id ASC;

-- name: CreateJob :one
-- Note: `id` is a required TEXT parameter (KSUID-backed); caller generates via types.NewJobID().
INSERT INTO jobs (
  id,
  run_id,
  repo_id,
  repo_base_ref,
  attempt,
  status,
  job_type,
  job_image,
  next_id,
  name,
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
  status,
  job_type,
  job_image,
  next_id,
  name,
  node_id,
  exit_code,
  started_at,
  finished_at,
  duration_ms,
  repo_sha_in,
  repo_sha_out,
  repo_sha_in8,
  repo_sha_out8,
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

-- name: CancelActiveJobsByRunAttempt :execrows
-- Bulk-cancels active jobs for a specific run attempt.
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
  AND attempt = $2
  AND status IN ('Created', 'Queued', 'Running');

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: ClaimJob :one
-- Atomically claim the next claimable job for a node.
WITH eligible AS (
  SELECT j.id, n.id AS node_id
  FROM nodes n
  JOIN jobs j ON TRUE
  JOIN runs r ON j.run_id = r.id
  JOIN waves w ON w.id = r.wave_id
  WHERE n.id = @node_id
    AND @node_id::TEXT != ''
    AND j.status = 'Queued'
    AND (j.node_id IS NULL OR j.node_id = n.id)
    AND r.status = 'Running'
    AND w.status = 'Started'
    AND NOT EXISTS (
      SELECT 1
      FROM jobs owner
      WHERE owner.run_id = j.run_id
        AND owner.attempt = j.attempt
        AND owner.node_id IS NOT NULL
        AND owner.node_id != n.id
  )
  ORDER BY j.run_id ASC, j.attempt ASC, j.id ASC
  FOR UPDATE OF j SKIP LOCKED
  LIMIT 1
)
UPDATE jobs
SET status = 'Running', node_id = eligible.node_id, started_at = now()
FROM eligible
WHERE jobs.id = eligible.id
RETURNING jobs.*;

-- name: UnclaimJob :exec
-- Revert a claimed Running job back to claimable Queued state.
-- Guarded by both job id and node id so a foreign node cannot steal the slot.
UPDATE jobs
SET status = 'Queued',
    node_id = CASE WHEN EXISTS (SELECT 1 FROM runs WHERE runs.id=jobs.run_id
      AND COALESCE((runs.stats->>'resume_count')::int, 0) > 0)
      THEN jobs.node_id ELSE NULL END,
    started_at = NULL
FROM (
  SELECT nodes.id AS node_id
  FROM nodes
  WHERE nodes.id = @node_id
    AND @node_id::TEXT != ''
) AS claiming_node
WHERE jobs.id = sqlc.arg(id)
  AND jobs.node_id = claiming_node.node_id
  AND status = 'Running';

-- name: ListStaleRunningJobs :many
-- Lists running jobs whose assigned node is stale at the provided cutoff.
-- Rows are grouped by (run_id, attempt) for deterministic recovery processing.
SELECT
  jobs.run_id,
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
GROUP BY jobs.run_id, jobs.attempt
ORDER BY jobs.run_id ASC, jobs.attempt ASC;

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
  jobs.status,
  jobs.job_type,
  jobs.job_image,
  jobs.next_id,
  jobs.name,
  jobs.node_id,
  jobs.exit_code,
  jobs.started_at,
  jobs.finished_at,
  jobs.duration_ms,
  jobs.repo_sha_in,
  jobs.repo_sha_out,
  jobs.repo_sha_in8,
  jobs.repo_sha_out8,
  jobs.meta;

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
      meta = sqlc.arg(meta)::jsonb
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

-- name: ListJobsPage :many
-- Lists jobs with optional run, node, and status filters, ordered newest-to-oldest by job id.
-- Joins runs and migs to surface mig_name for CLI and TUI consumers.
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
  AND (sqlc.narg(node_id)::text IS NULL OR jobs.node_id = sqlc.narg(node_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR jobs.status::text = sqlc.narg(status)::text)
ORDER BY jobs.id DESC
LIMIT $1 OFFSET $2;

-- name: CountJobsPage :one
-- Counts jobs matching the same optional filters as ListJobsPage.
SELECT COUNT(jobs.id)::BIGINT
FROM jobs
JOIN runs ON jobs.run_id = runs.id
JOIN migs ON runs.mig_id = migs.id
WHERE (sqlc.narg(run_id)::text IS NULL OR jobs.run_id = sqlc.narg(run_id)::text)
  AND (sqlc.narg(node_id)::text IS NULL OR jobs.node_id = sqlc.narg(node_id)::text)
  AND (sqlc.narg(status)::text IS NULL OR jobs.status::text = sqlc.narg(status)::text);
