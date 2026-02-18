-- name: GetLog :one
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE id = $1;

-- name: ListLogsByRun :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunSince :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND id > $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJob :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND job_id = $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJobSince :many
-- Returns log metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND job_id = $2 AND id > $3
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

-- name: ListLogsMetaByRun :many
-- Returns log metadata for a run.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsMetaByRunSince :many
-- Returns log metadata for a run since a given id.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND id > $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsMetaByRunAndJob :many
-- Returns log metadata for a run and job.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND job_id = $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsMetaByRunAndJobSince :many
-- Returns log metadata for a run and job since a given id.
SELECT id, run_id, job_id, chunk_no, data_size, object_key, created_at FROM logs
WHERE run_id = $1 AND job_id = $2 AND id > $3
ORDER BY chunk_no ASC, id ASC;
