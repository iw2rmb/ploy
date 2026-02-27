-- name: CreatePrepRun :one
INSERT INTO prep_runs (repo_id, attempt, status, started_at, result_json, logs_ref)
VALUES ($1, $2, $3, now(), $4, $5)
RETURNING repo_id, attempt, status, started_at, finished_at, result_json, logs_ref;

-- name: FinishPrepRun :one
UPDATE prep_runs
SET status = $3,
    finished_at = now(),
    result_json = $4,
    logs_ref = $5
WHERE repo_id = $1
  AND attempt = $2
RETURNING repo_id, attempt, status, started_at, finished_at, result_json, logs_ref;

-- name: ListPrepRunsByRepo :many
SELECT repo_id, attempt, status, started_at, finished_at, result_json, logs_ref
FROM prep_runs
WHERE repo_id = $1
ORDER BY attempt DESC;
