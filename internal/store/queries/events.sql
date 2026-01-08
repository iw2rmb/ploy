-- name: GetEvent :one
SELECT * FROM events
WHERE id = $1;

-- name: ListEventsByRun :many
SELECT * FROM events
WHERE run_id = $1
ORDER BY time ASC, id ASC;

-- name: ListEventsByRunSince :many
SELECT * FROM events
WHERE run_id = $1 AND id > $2
ORDER BY time ASC, id ASC;

-- name: CreateEvent :one
INSERT INTO events (run_id, job_id, time, level, message, meta)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListEventsMetaByRun :many
-- Returns event metadata (without the meta blob) for a run.
-- Use GetEvent to fetch the full event with meta by id.
SELECT id, run_id, job_id, time, level, message FROM events
WHERE run_id = $1
ORDER BY time ASC, id ASC;

-- name: ListEventsMetaByRunSince :many
-- Returns event metadata (without the meta blob) for a run since a given id.
-- Use GetEvent to fetch the full event with meta by id.
SELECT id, run_id, job_id, time, level, message FROM events
WHERE run_id = $1 AND id > $2
ORDER BY time ASC, id ASC;
