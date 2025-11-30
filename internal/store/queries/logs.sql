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

-- name: ListLogsByRunJobAndBuild :many
SELECT * FROM logs
WHERE run_id = $1 AND job_id = $2 AND build_id = $3
ORDER BY chunk_no ASC, id ASC;

-- name: ListLogsByRunJobAndBuildSince :many
SELECT * FROM logs
WHERE run_id = $1 AND job_id = $2 AND build_id = $3 AND id > $4
ORDER BY chunk_no ASC, id ASC;

-- name: CreateLog :one
INSERT INTO logs (run_id, job_id, build_id, chunk_no, data)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteLog :exec
DELETE FROM logs
WHERE id = $1;

-- name: DeleteLogsOlderThan :exec
DELETE FROM logs
WHERE created_at < $1;
