-- name: GetRun :one
SELECT * FROM runs
WHERE id = $1;

-- name: ListRuns :many
SELECT * FROM runs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateRun :one
INSERT INTO runs (repo_url, spec, created_by, status, base_ref, target_ref, commit_sha)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2, reason = $3, finished_at = $4
WHERE id = $1;

-- name: ClaimRun :one
-- Claims a queued run for a node. Only claims runs that do NOT have run_steps rows.
-- Multi-step runs (with run_steps entries) must be claimed via ClaimRunStep instead.
WITH cte AS (
  SELECT runs.id FROM runs
  INNER JOIN nodes ON nodes.id = $1
  WHERE runs.status = 'queued'
    AND nodes.drained = false
    -- Exclude runs that have run_steps: multi-step runs are claimed via ClaimRunStep.
    AND NOT EXISTS (
      SELECT 1 FROM run_steps
      WHERE run_steps.run_id = runs.id
    )
  ORDER BY runs.created_at
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE runs r
SET status = 'assigned', node_id = $1, started_at = now()
FROM cte
WHERE r.id = cte.id
RETURNING r.*;

-- name: AckRunStart :exec
UPDATE runs
SET status = 'running'
WHERE id = $1 AND status = 'assigned';

-- name: UpdateRunCompletion :exec
UPDATE runs
SET status = $2, reason = $3, finished_at = now(), stats = $4
WHERE id = $1;

-- name: DeleteRun :exec
DELETE FROM runs
WHERE id = $1;

-- name: GetRunTiming :one
SELECT id,
       COALESCE(queue_ms, 0) AS queue_ms,
       COALESCE(run_ms, 0)   AS run_ms
FROM runs_timing
WHERE id = $1;

-- name: ListRunsTimings :many
SELECT id,
       COALESCE(queue_ms, 0) AS queue_ms,
       COALESCE(run_ms, 0)   AS run_ms
FROM runs_timing
ORDER BY id DESC
LIMIT $1 OFFSET $2;

