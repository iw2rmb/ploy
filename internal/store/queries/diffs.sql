-- name: GetDiff :one
SELECT * FROM diffs
WHERE id = $1;

-- name: ListDiffsByRun :many
-- Returns diffs for a run ordered by job step_index, then by created_at.
-- Joins with jobs to get ordering from job's step_index.
SELECT d.* FROM diffs d
LEFT JOIN jobs j ON d.job_id = j.id
WHERE d.run_id = $1
ORDER BY j.step_index NULLS LAST, d.created_at ASC;

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
