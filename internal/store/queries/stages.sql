-- name: GetStage :one
SELECT * FROM stages
WHERE id = $1;

-- name: ListStagesByRun :many
SELECT * FROM stages
WHERE run_id = $1
ORDER BY name ASC;

-- name: CreateStage :one
INSERT INTO stages (run_id, name, status, meta)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateStageStatus :exec
UPDATE stages
SET status = $2, started_at = $3, finished_at = $4, duration_ms = $5
WHERE id = $1;

-- name: DeleteStage :exec
DELETE FROM stages
WHERE id = $1;
