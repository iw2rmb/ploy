-- name: GetDiff :one
-- Returns diff metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, patch_size, object_key, summary, created_at FROM diffs
WHERE id = $1;

-- name: ListDiffsBeforeStep :many
-- Returns all diffs for a run up to (and including) the specified next_id.
-- next_id is read from summary metadata when present.
SELECT d.id, d.run_id, d.job_id, d.patch_size, d.object_key, d.summary, d.created_at FROM diffs d
WHERE d.run_id = $1
  AND (
    CASE
      WHEN jsonb_typeof(d.summary->'next_id') = 'number' THEN (d.summary->>'next_id')::DOUBLE PRECISION
      ELSE 0
    END
  ) <= $2
ORDER BY
  CASE
    WHEN jsonb_typeof(d.summary->'next_id') = 'number' THEN (d.summary->>'next_id')::DOUBLE PRECISION
    ELSE 0
  END ASC,
  d.created_at ASC,
  d.id ASC;

-- name: CreateDiff :one
-- Creates a new diff entry associated with a job. Blob data is stored in object storage.
INSERT INTO diffs (run_id, job_id, patch_size, summary)
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
SELECT d.id, d.run_id, d.job_id, d.patch_size, d.object_key, d.summary, d.created_at FROM diffs d
JOIN jobs j ON j.id = d.job_id
WHERE d.run_id = $1 AND j.repo_id = $2
ORDER BY
  CASE
    WHEN jsonb_typeof(d.summary->'next_id') = 'number' THEN (d.summary->>'next_id')::DOUBLE PRECISION
    ELSE 0
  END ASC,
  d.created_at ASC,
  d.id ASC;

-- name: ListDiffsMetaByRun :many
-- Returns diff metadata for a run.
SELECT id, run_id, job_id, patch_size, object_key, summary, created_at FROM diffs
WHERE run_id = $1
ORDER BY created_at ASC, id ASC;

-- name: ListDiffsMetaByRunRepo :many
-- Returns diff metadata for a specific repo within a run.
SELECT d.id, d.run_id, d.job_id, d.patch_size, d.object_key, d.summary, d.created_at FROM diffs d
JOIN jobs j ON j.id = d.job_id
WHERE d.run_id = $1 AND j.repo_id = $2
ORDER BY
  CASE
    WHEN jsonb_typeof(d.summary->'next_id') = 'number' THEN (d.summary->>'next_id')::DOUBLE PRECISION
    ELSE 0
  END ASC,
  d.created_at ASC,
  d.id ASC;
