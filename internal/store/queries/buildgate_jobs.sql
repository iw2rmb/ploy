-- name: GetBuildGateJob :one
SELECT * FROM buildgate_jobs
WHERE id = $1;

-- name: CreateBuildGateJob :one
INSERT INTO buildgate_jobs (request_payload, status)
VALUES ($1, 'pending')
RETURNING *;

-- name: ClaimBuildGateJob :one
WITH cte AS (
  SELECT buildgate_jobs.id FROM buildgate_jobs
  INNER JOIN nodes ON nodes.id = $1
  WHERE buildgate_jobs.status = 'pending' AND nodes.drained = false
  ORDER BY buildgate_jobs.created_at
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE buildgate_jobs bg
SET status = 'claimed', node_id = $1, started_at = now()
FROM cte
WHERE bg.id = cte.id
RETURNING bg.*;

-- name: AckBuildGateJobStart :exec
UPDATE buildgate_jobs
SET status = 'running'
WHERE id = $1 AND status = 'claimed';

-- name: UpdateBuildGateJobCompletion :exec
UPDATE buildgate_jobs
SET status = $2, result = $3, error = $4, finished_at = now()
WHERE id = $1;

-- name: ListPendingBuildGateJobs :many
SELECT * FROM buildgate_jobs
WHERE status = 'pending'
ORDER BY created_at
LIMIT $1 OFFSET $2;
