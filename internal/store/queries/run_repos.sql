-- name: CreateRunRepo :one
-- Creates a new run_repo entry for batched runs.
-- Each run_repo represents one repository within a batch (parent run).
INSERT INTO run_repos (run_id, repo_url, base_ref, target_ref, status)
VALUES ($1, $2, $3, $4, 'pending')
RETURNING *;

-- name: GetRunRepo :one
SELECT * FROM run_repos
WHERE id = $1;

-- name: ListRunReposByRun :many
-- Lists all repos associated with a run (batch), ordered by creation time.
SELECT * FROM run_repos
WHERE run_id = $1
ORDER BY created_at ASC;

-- name: UpdateRunRepoStatus :exec
-- Updates a run_repo's status along with timing fields.
-- started_at is set when transitioning to 'running'.
-- finished_at is set when transitioning to a terminal status.
UPDATE run_repos
SET status = $2,
    started_at = CASE WHEN $2 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN $2 IN ('succeeded', 'failed', 'skipped', 'cancelled') THEN now() ELSE finished_at END
WHERE id = $1;

-- name: UpdateRunRepoError :exec
-- Updates a run_repo's last_error field (e.g., on failure).
UPDATE run_repos
SET last_error = $2
WHERE id = $1;

-- name: IncrementRunRepoAttempt :exec
-- Increments the attempt counter and resets status to 'pending' for retry.
-- Clears timing fields to prepare for a fresh execution attempt.
UPDATE run_repos
SET attempt = attempt + 1,
    status = 'pending',
    last_error = NULL,
    started_at = NULL,
    finished_at = NULL
WHERE id = $1;

-- name: CountRunReposByStatus :many
-- Aggregates run_repos counts by status for a given run.
-- Used to derive batch-level status (e.g., all succeeded = batch succeeded).
SELECT status, COUNT(*)::int AS count
FROM run_repos
WHERE run_id = $1
GROUP BY status;

-- name: DeleteRunRepo :exec
DELETE FROM run_repos
WHERE id = $1;
