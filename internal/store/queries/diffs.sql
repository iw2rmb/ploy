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
ORDER BY j.step_index ASC, d.created_at ASC, d.id ASC;

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
-- Repo attribution comes from joining diffs.job_id to jobs.repo_id.
-- This supports the repo-scoped endpoint GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
-- Diffs for repo A are excluded from repo B listing via the j.repo_id filter.
SELECT d.* FROM diffs d
JOIN jobs j ON j.id = d.job_id
WHERE d.run_id = $1 AND j.repo_id = $2
ORDER BY j.step_index ASC, d.created_at ASC, d.id ASC;

-- name: ListDiffsMetaByRun :many
-- Returns diff metadata (without the patch blob) for a run.
-- Use GetDiff to fetch the actual patch data by id.
SELECT id, run_id, job_id, summary, created_at, octet_length(patch)::BIGINT AS patch_size FROM diffs
WHERE run_id = $1
ORDER BY created_at ASC, id ASC;

-- name: ListDiffsMetaByRunRepo :many
-- Returns diff metadata (without the patch blob) for a specific repo within a run.
-- Use GetDiff to fetch the actual patch data by id.
SELECT d.id, d.run_id, d.job_id, d.summary, d.created_at, octet_length(patch)::BIGINT AS patch_size FROM diffs d
JOIN jobs j ON j.id = d.job_id
WHERE d.run_id = $1 AND j.repo_id = $2
ORDER BY j.step_index ASC, d.created_at ASC, d.id ASC;
