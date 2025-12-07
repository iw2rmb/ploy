-- name: CreateRunRepo :one
-- Creates a new run_repo entry for batched runs.
-- Each run_repo represents one repository within a batch (parent run).
-- The id parameter is a NanoID-backed string generated via NewRunRepoID().
INSERT INTO run_repos (id, run_id, repo_url, base_ref, target_ref, status)
VALUES ($1, $2, $3, $4, $5, 'pending')
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
-- Clears timing fields and execution_run_id to prepare for a fresh execution attempt.
UPDATE run_repos
SET attempt = attempt + 1,
    status = 'pending',
    last_error = NULL,
    execution_run_id = NULL,
    started_at = NULL,
    finished_at = NULL
WHERE id = $1;

-- name: UpdateRunRepoRefs :exec
-- Updates a run_repo's base_ref and target_ref (e.g., when restarting with new refs).
UPDATE run_repos
SET base_ref = $2,
    target_ref = $3
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

-- name: SetRunRepoExecutionRun :exec
-- Links a run_repo to its child execution run and transitions status to 'running'.
-- Called when starting execution for a repo entry within a batch.
UPDATE run_repos
SET execution_run_id = $2,
    status = 'running',
    started_at = CASE WHEN started_at IS NULL THEN now() ELSE started_at END
WHERE id = $1;

-- name: ListPendingRunReposByRun :many
-- Lists all pending repos for a run (batch), ordered by creation time.
-- Used by the batch orchestrator to find repos ready to start execution.
SELECT * FROM run_repos
WHERE run_id = $1 AND status = 'pending'
ORDER BY created_at ASC;

-- name: GetRunRepoByExecutionRun :one
-- Finds the run_repo entry linked to a given execution run.
-- Used by completion callbacks to update repo status when execution completes.
SELECT * FROM run_repos
WHERE execution_run_id = $1;

-- name: ClearRunRepoExecutionRun :exec
-- Clears the execution_run_id for a run_repo (e.g., when restarting).
-- Also called by IncrementRunRepoAttempt to prepare for a new execution.
UPDATE run_repos
SET execution_run_id = NULL
WHERE id = $1;

-- name: ListBatchRunsWithPendingRepos :many
-- Lists batch runs that have at least one pending run_repo entry.
-- Used by the batch scheduler to find runs that need repos to be started.
-- Returns distinct run IDs for runs in non-terminal states (queued, assigned, running)
-- that have pending repos ready for execution.
SELECT DISTINCT r.id
FROM runs r
INNER JOIN run_repos rr ON r.id = rr.run_id
WHERE r.status IN ('queued', 'assigned', 'running')
  AND rr.status = 'pending'
ORDER BY r.id;

-- name: ListDistinctRepos :many
-- Lists distinct repository URLs from run_repos with optional substring filter.
-- Returns repo_url along with the most recent run timestamp and status for each repo.
-- Used by GET /v1/repos to provide a repo-centric view of batch activity.
SELECT DISTINCT ON (rr.repo_url)
    rr.repo_url,
    rr.started_at AS last_run_at,
    rr.status AS last_status
FROM run_repos rr
WHERE
    -- Optional substring filter: if @filter is NULL or empty, match all.
    (@filter::text IS NULL OR @filter = '' OR rr.repo_url ILIKE '%' || @filter || '%')
ORDER BY rr.repo_url, rr.started_at DESC NULLS LAST;

-- name: ListRunsForRepo :many
-- Lists all runs (via run_repos) for a given repository URL.
-- Returns run details joined with run_repo status and timing for repo-centric view.
-- Used by GET /v1/repos/{repo_id}/runs to show run history for a specific repo.
SELECT
    r.id AS run_id,
    r.name,
    r.status AS run_status,
    rr.status AS repo_status,
    rr.base_ref,
    rr.target_ref,
    rr.attempt,
    rr.started_at,
    rr.finished_at
FROM run_repos rr
INNER JOIN runs r ON rr.run_id = r.id
WHERE rr.repo_url = @repo_url
ORDER BY rr.created_at DESC
LIMIT @lim
OFFSET @off;
