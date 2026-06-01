-- name: CreateRunAction :one
INSERT INTO run_actions (
  id,
  run_id,
  attempt,
  action_type,
  status,
  meta
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRunAction :one
SELECT *
FROM run_actions
WHERE id = $1;

-- name: GetRunActionByKey :one
SELECT *
FROM run_actions
WHERE run_id = $1
  AND attempt = $2
  AND action_type = $3;

-- name: ClaimRunAction :one
WITH eligible AS (
  SELECT a.id, n.id AS node_id
  FROM nodes n
  JOIN run_actions a ON TRUE
  JOIN runs r ON r.id = a.run_id
  WHERE n.id = @node_id
    AND @node_id::TEXT != ''
    AND a.status = 'Queued'
    AND a.node_id IS NULL
    AND r.status IN ('Success', 'Fail', 'Cancelled')
  ORDER BY a.run_id ASC, a.attempt ASC, a.id ASC
  FOR UPDATE OF a SKIP LOCKED
  LIMIT 1
)
UPDATE run_actions a
SET status = 'Running',
    node_id = eligible.node_id,
    started_at = now()
FROM eligible
WHERE a.id = eligible.id
RETURNING a.*;

-- name: UpdateRunActionCompletion :exec
UPDATE run_actions
SET status = $2,
    finished_at = now(),
    duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT,
    meta = $3
WHERE id = $1;

-- name: ListRunActionsByRunAttempt :many
SELECT *
FROM run_actions
WHERE run_id = $1
  AND attempt = $2
ORDER BY id ASC;

-- name: HasRunningActionForRunNode :one
SELECT EXISTS (
  SELECT 1
  FROM run_actions
  WHERE run_id = $1
    AND node_id = $2
    AND status = 'Running'
)::boolean;
