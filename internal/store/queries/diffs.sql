-- name: CreateDiff :one
-- Creates a new diff entry associated with a job. Blob data is stored in object storage.
INSERT INTO diffs (run_id, job_id, patch_size, summary)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteDiff :exec
DELETE FROM diffs
WHERE id = $1;

-- name: DeleteDiffsOlderThan :exec
DELETE FROM diffs
WHERE created_at < $1;

-- name: ListDiffsByRun :many
-- Returns diff metadata for a run.
SELECT
  id,
  run_id,
  job_id,
  patch_size,
  object_key,
  summary,
  created_at
FROM diffs
WHERE run_id = sqlc.arg(run_id)
ORDER BY created_at ASC, id ASC;

-- name: GetLatestDiffByJob :one
SELECT id, run_id, job_id, patch_size, object_key, summary, created_at
FROM diffs
WHERE job_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;
