-- name: CreateRunRepo :one
-- v1: Creates a new run_repos row scoped to (run_id, repo_id).
-- Note: attempt defaults to 1; status defaults to 'Queued'.
INSERT INTO run_repos (mod_id, run_id, repo_id, repo_base_ref, repo_target_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING mod_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, attempt, last_error, created_at, started_at, finished_at;

-- name: GetRunRepo :one
SELECT mod_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, attempt, last_error, created_at, started_at, finished_at
FROM run_repos
WHERE run_id = $1 AND repo_id = $2;

-- name: ListRunReposByRun :many
-- Lists all repos associated with a run, ordered by creation time.
SELECT mod_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, attempt, last_error, created_at, started_at, finished_at
FROM run_repos
WHERE run_id = $1
ORDER BY created_at ASC, repo_id ASC;

-- name: UpdateRunRepoStatus :exec
-- Updates repo status + timing fields.
-- started_at: set when transitioning to Running.
-- finished_at: set when transitioning to a terminal status.
UPDATE run_repos
SET status = $3,
    started_at = CASE WHEN $3 = 'Running'::run_repo_status AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN $3 IN ('Success'::run_repo_status, 'Fail'::run_repo_status, 'Cancelled'::run_repo_status) THEN COALESCE(finished_at, now()) ELSE finished_at END
WHERE run_id = $1 AND repo_id = $2;

-- name: UpdateRunRepoError :exec
UPDATE run_repos
SET last_error = $3
WHERE run_id = $1 AND repo_id = $2;

-- name: IncrementRunRepoAttempt :exec
-- Increments attempt and resets status/timing for a fresh repo execution attempt.
UPDATE run_repos
SET attempt = attempt + 1,
    status = 'Queued',
    last_error = NULL,
    started_at = NULL,
    finished_at = NULL
WHERE run_id = $1 AND repo_id = $2;

-- name: UpdateRunRepoRefs :exec
-- Updates snapshot refs for the run repo (used when restarting with new refs).
UPDATE run_repos
SET repo_base_ref = $3,
    repo_target_ref = $4
WHERE run_id = $1 AND repo_id = $2;

-- name: CountRunReposByStatus :many
SELECT status, COUNT(*)::int AS count
FROM run_repos
WHERE run_id = $1
GROUP BY status;

-- name: CancelActiveRunReposByRun :execrows
-- Bulk-cancels active repos for a run (Queued/Running -> Cancelled).
UPDATE run_repos
SET status = 'Cancelled',
    finished_at = COALESCE(finished_at, now())
WHERE run_id = $1
  AND status IN ('Queued', 'Running');

-- name: DeleteRunRepo :exec
DELETE FROM run_repos
WHERE run_id = $1 AND repo_id = $2;

-- name: ListQueuedRunReposByRun :many
SELECT mod_id, run_id, repo_id, repo_base_ref, repo_target_ref, status, attempt, last_error, created_at, started_at, finished_at
FROM run_repos
WHERE run_id = $1 AND status = 'Queued'
ORDER BY created_at ASC, repo_id ASC;

-- name: ListRunsWithQueuedRepos :many
-- Lists runs that have queued work (at least one Queued run_repos row).
SELECT DISTINCT r.id
FROM runs r
JOIN run_repos rr ON r.id = rr.run_id
WHERE r.status = 'Started'
  AND rr.status = 'Queued'
ORDER BY r.id;

-- name: ListRunsForRepo :many
-- Lists runs for a given repo_id (mod_repos.id).
SELECT
  r.id AS run_id,
  r.mod_id,
  r.status AS run_status,
  rr.status AS repo_status,
  rr.repo_base_ref,
  rr.repo_target_ref,
  rr.attempt,
  rr.started_at,
  rr.finished_at
FROM run_repos rr
JOIN runs r ON rr.run_id = r.id
WHERE rr.repo_id = $1
ORDER BY rr.created_at DESC, rr.run_id DESC
LIMIT $2 OFFSET $3;

-- name: ListFailedRepoIDsByMod :many
-- Lists repo_ids whose last terminal run_repos status is 'Fail' for a given mod.
-- "Last terminal state" per repo_id is determined by looking at the newest run_repos
-- row where status in (Fail, Success, Cancelled) and selecting those where status='Fail'.
-- Uses a subquery to get the last terminal status per repo, then filters for 'Fail'.
SELECT repo_id FROM (
  SELECT DISTINCT ON (rr.repo_id) rr.repo_id, rr.status
  FROM run_repos rr
  WHERE rr.mod_id = $1
    AND rr.status IN ('Fail', 'Success', 'Cancelled')
  ORDER BY rr.repo_id, rr.created_at DESC, rr.run_id DESC
) AS last_status
WHERE status = 'Fail';

-- name: ListRunReposWithURLByRun :many
-- v1: Lists all run_repos for a run with their repo_url (from mod_repos).
-- Used by:
-- - GET  /v1/runs/{id}/repos (full repo response without N+1 lookups)
-- - POST /v1/runs/{run_id}/pull (repo resolution by normalized URL)
SELECT rr.mod_id, rr.run_id, rr.repo_id, rr.repo_base_ref, rr.repo_target_ref,
       rr.status, rr.attempt, rr.last_error, rr.created_at, rr.started_at, rr.finished_at,
       mr.repo_url
FROM run_repos rr
JOIN mod_repos mr ON rr.repo_id = mr.id
WHERE rr.run_id = $1
ORDER BY rr.created_at ASC, rr.repo_id ASC;

-- name: GetLatestRunRepoByModAndRepoStatus :one
-- v1: Gets the newest run_repos row for a specific repo_id in a mod,
-- filtered by terminal status (Success or Fail).
-- Used by POST /v1/mods/{mod_id}/pull to select last-succeeded or last-failed.
-- Order by created_at DESC to get the newest matching run_repos row.
SELECT rr.run_id, rr.repo_id, rr.repo_target_ref
FROM run_repos rr
JOIN runs r ON rr.run_id = r.id
WHERE r.mod_id = $1
  AND rr.repo_id = $2
  AND rr.status = $3
ORDER BY rr.created_at DESC
LIMIT 1;
