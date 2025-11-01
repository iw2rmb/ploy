-- name: GetRun :one
SELECT * FROM runs
WHERE id = $1;

-- name: ListRuns :many
SELECT * FROM runs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListRunsByMod :many
SELECT * FROM runs
WHERE mod_id = $1
ORDER BY created_at DESC;

-- name: CreateRun :one
INSERT INTO runs (mod_id, status, base_ref, target_ref, commit_sha)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2, reason = $3, finished_at = $4
WHERE id = $1;

-- name: ClaimRun :one
WITH cte AS (
  SELECT id FROM runs
  WHERE status = 'queued'
  ORDER BY created_at
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
