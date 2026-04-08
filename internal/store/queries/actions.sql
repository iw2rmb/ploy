-- name: CreateRunRepoAction :one
INSERT INTO run_repo_actions (
  id,
  run_id,
  repo_id,
  attempt,
  action_type,
  status,
  meta
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetRunRepoAction :one
SELECT *
FROM run_repo_actions
WHERE id = $1;

-- name: GetRunRepoActionByKey :one
SELECT *
FROM run_repo_actions
WHERE run_id = $1
  AND repo_id = $2
  AND attempt = $3
  AND action_type = $4;

-- name: ClaimRunRepoAction :one
WITH eligible AS (
  SELECT a.id, n.id AS node_id
  FROM nodes n
  JOIN run_repo_actions a ON TRUE
  JOIN runs r ON r.id = a.run_id
  WHERE n.id = @node_id
    AND @node_id::TEXT != ''
    AND a.status = 'Queued'
    AND a.node_id IS NULL
    AND r.status = 'Finished'
  ORDER BY a.run_id ASC, a.repo_id ASC, a.attempt ASC, a.id ASC
  FOR UPDATE OF a SKIP LOCKED
  LIMIT 1
)
UPDATE run_repo_actions a
SET status = 'Running',
    node_id = eligible.node_id,
    started_at = now()
FROM eligible
WHERE a.id = eligible.id
RETURNING a.*;

-- name: UpdateRunRepoActionCompletion :exec
UPDATE run_repo_actions
SET status = $2,
    finished_at = now(),
    duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT,
    meta = $3
WHERE id = $1;

-- name: ListRunRepoActionsByRunRepoAttempt :many
SELECT *
FROM run_repo_actions
WHERE run_id = $1
  AND repo_id = $2
  AND attempt = $3
ORDER BY id ASC;

