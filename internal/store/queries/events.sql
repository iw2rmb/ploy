-- name: GetEvent :one
SELECT * FROM events
WHERE id = $1;

-- name: ListEventsByRun :many
SELECT id, run_id, job_id, time, level, message, meta FROM events
WHERE run_id = sqlc.arg(run_id)
ORDER BY time ASC, id ASC;

-- name: ListEventsByRunSince :many
SELECT id, run_id, job_id, time, level, message, meta FROM events
WHERE run_id = sqlc.arg(run_id) AND id > sqlc.arg(id)
ORDER BY time ASC, id ASC;

-- name: CreateEvent :one
INSERT INTO events (run_id, job_id, time, level, message, meta)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
