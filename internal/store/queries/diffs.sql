-- name: GetDiff :one
SELECT * FROM diffs
WHERE id = $1;

-- name: ListDiffsByRun :many
SELECT * FROM diffs
WHERE run_id = $1
ORDER BY created_at DESC;

-- name: CreateDiff :one
INSERT INTO diffs (run_id, stage_id, patch, summary)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteDiff :exec
DELETE FROM diffs
WHERE id = $1;

-- name: DeleteDiffsOlderThan :exec
DELETE FROM diffs
WHERE created_at < $1;
