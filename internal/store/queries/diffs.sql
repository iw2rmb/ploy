-- name: GetDiff :one
SELECT * FROM diffs
WHERE id = $1;

-- name: ListDiffsBeforeStep :many
-- Returns all diffs for a run up to (and including) the specified step_index.
-- Used for workspace rehydration: apply all diffs from jobs with step_index <= k to build workspace for step k+1.
-- Excludes diffs without associated jobs (NULL job_id) to avoid applying orphan diffs during rehydration.
SELECT d.* FROM diffs d
INNER JOIN jobs j ON d.job_id = j.id
WHERE d.run_id = $1
  AND j.step_index <= $2
ORDER BY j.step_index ASC, d.created_at ASC;

-- name: CreateDiff :one
-- Creates a new diff entry associated with a job.
-- Ordering is determined by the job's step_index.
INSERT INTO diffs (run_id, job_id, patch, summary)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteDiff :exec
DELETE FROM diffs
WHERE id = $1;

-- name: DeleteDiffsOlderThan :exec
DELETE FROM diffs
WHERE created_at < $1;

-- name: ListDiffsByRunRepo :many
-- Returns diffs for a specific repo execution within a run.
-- Per roadmap/v1/scope.md:85 and roadmap/v1/api.md:263, repo attribution comes from
-- joining diffs.job_id → jobs.repo_id. This is the v1 repo-scoped endpoint for
-- GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
-- Diffs for repo A are excluded from repo B listing via the j.repo_id filter.
SELECT d.* FROM diffs d
JOIN jobs j ON j.id = d.job_id
WHERE d.run_id = $1 AND j.repo_id = $2
ORDER BY j.step_index ASC, d.created_at ASC;
