-- name: GetDiff :one
SELECT * FROM diffs
WHERE id = $1;

-- name: ListDiffsByRun :many
-- Returns diffs for a run ordered by step_index (if present), then by created_at.
-- This allows rehydration to select diffs in logical step order for multi-step runs.
SELECT * FROM diffs
WHERE run_id = $1
ORDER BY
  step_index NULLS LAST,  -- NULL step_index (legacy diffs) appear last
  created_at DESC;

-- name: ListDiffsBeforeStep :many
-- Returns all diffs for a run up to (and including) the specified step_index.
-- Used for workspace rehydration: apply all diffs from steps 0..k to build workspace for step k+1.
-- Excludes diffs with NULL step_index to avoid applying legacy/aggregate diffs during rehydration.
SELECT * FROM diffs
WHERE run_id = $1
  AND step_index IS NOT NULL
  AND step_index <= $2
ORDER BY step_index ASC, created_at ASC;

-- name: CreateDiff :one
-- Creates a new diff entry with optional step_index for multi-step runs.
-- step_index is NULL for legacy single-step runs or final aggregate diffs.
INSERT INTO diffs (run_id, stage_id, patch, summary, step_index)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteDiff :exec
DELETE FROM diffs
WHERE id = $1;

-- name: DeleteDiffsOlderThan :exec
DELETE FROM diffs
WHERE created_at < $1;
