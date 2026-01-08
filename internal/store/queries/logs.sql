-- name: GetLog :one
SELECT * FROM logs
WHERE id = $1;

-- name: ListLogsByRun :many
SELECT * FROM logs
WHERE run_id = $1
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunSince :many
SELECT * FROM logs
WHERE run_id = $1 AND id > $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJob :many
SELECT * FROM logs
WHERE run_id = $1 AND job_id = $2
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunAndJobSince :many
SELECT * FROM logs
WHERE run_id = $1 AND job_id = $2 AND id > $3
ORDER BY chunk_no ASC, id ASC;

-- name: CreateLog :one
-- Creates a new log chunk. Logs are grouped at the job level only (build_id removed).
INSERT INTO logs (run_id, job_id, chunk_no, data)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteLog :exec
DELETE FROM logs
WHERE id = $1;

-- name: DeleteLogsOlderThan :exec
DELETE FROM logs
WHERE created_at < $1;

-- name: ListLogsMetaByRun :many
-- Returns log metadata (without the data blob) for a run.
-- Use GetLog to fetch the actual log data by id.
SELECT id, run_id, job_id, chunk_no, created_at FROM logs
WHERE run_id = $1
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsMetaByRunAndJob :many
-- Returns log metadata (without the data blob) for a run and job.
-- Use GetLog to fetch the actual log data by id.
SELECT id, run_id, job_id, chunk_no, created_at FROM logs
WHERE run_id = $1 AND job_id = $2
ORDER BY chunk_no ASC, id ASC;
