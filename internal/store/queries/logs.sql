-- name: GetLog :one
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE id = $1;

-- name: ListLogsByRun :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT
  id,
  run_id,
  job_id,
  chunk_no,
  data_size,
  object_key,
  created_at
FROM logs
WHERE run_id = sqlc.arg(run_id)
  AND (sqlc.arg(metadata_only)::boolean OR NOT sqlc.arg(metadata_only)::boolean)
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunSince :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT
  id,
  run_id,
  job_id,
  chunk_no,
  data_size,
  object_key,
  created_at
FROM logs
WHERE run_id = sqlc.arg(run_id) AND id > sqlc.arg(id)
  AND (sqlc.arg(metadata_only)::boolean OR NOT sqlc.arg(metadata_only)::boolean)
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJob :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT
  id,
  run_id,
  job_id,
  chunk_no,
  data_size,
  object_key,
  created_at
FROM logs
WHERE run_id = sqlc.arg(run_id) AND job_id = sqlc.arg(job_id)
  AND (sqlc.arg(metadata_only)::boolean OR NOT sqlc.arg(metadata_only)::boolean)
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJobSince :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT
  id,
  run_id,
  job_id,
  chunk_no,
  data_size,
  object_key,
  created_at
FROM logs
WHERE run_id = sqlc.arg(run_id) AND job_id = sqlc.arg(job_id) AND id > sqlc.arg(id)
  AND (sqlc.arg(metadata_only)::boolean OR NOT sqlc.arg(metadata_only)::boolean)
ORDER BY chunk_no ASC, id ASC;

-- name: CreateLog :one
-- Creates a new log chunk metadata. Blob data is stored in object storage.
-- Logs are grouped at the job level only (build_id removed).
INSERT INTO logs (run_id, job_id, chunk_no, data_size)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteLog :exec
DELETE FROM logs
WHERE id = $1;

-- name: DeleteLogsOlderThan :exec
DELETE FROM logs
WHERE created_at < $1;
