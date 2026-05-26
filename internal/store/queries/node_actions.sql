-- name: CreateNodeAction :one
INSERT INTO node_actions (
  id,
  node_id,
  action_type,
  status,
  meta
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetNodeAction :one
SELECT *
FROM node_actions
WHERE id = $1;

-- name: ClaimNodeAction :one
WITH eligible AS (
  SELECT a.id
  FROM node_actions a
  WHERE a.node_id = @node_id
    AND @node_id::TEXT != ''
    AND a.status = 'Queued'
  ORDER BY a.id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE node_actions a
SET status = 'Running',
    started_at = now()
FROM eligible
WHERE a.id = eligible.id
RETURNING a.*;

-- name: UpdateNodeActionCompletion :exec
UPDATE node_actions
SET status = $2,
    finished_at = now(),
    duration_ms = COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT,
    result = $3
WHERE id = $1;

-- name: ListNodeActions :many
SELECT *
FROM node_actions
WHERE node_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;
