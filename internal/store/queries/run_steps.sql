-- name: GetRunStep :one
-- Retrieves a single step by its ID.
SELECT * FROM run_steps
WHERE id = $1;

-- name: GetRunStepByIndex :one
-- Retrieves a specific step of a run by step_index.
SELECT * FROM run_steps
WHERE run_id = $1 AND step_index = $2;

-- name: ListRunSteps :many
-- Lists all steps for a run ordered by step_index.
SELECT * FROM run_steps
WHERE run_id = $1
ORDER BY step_index ASC;

-- name: CreateRunStep :one
-- Creates a new step for a run (called when multi-step run is queued).
INSERT INTO run_steps (run_id, step_index, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ClaimRunStep :one
-- Claims the next available step for execution using FOR UPDATE SKIP LOCKED.
-- Returns the step and its parent run information for execution.
--
-- Claim strategy:
-- 1. Find queued steps where the previous step (step_index - 1) has succeeded OR step_index = 0.
-- 2. Join with runs to get run metadata (repo_url, base_ref, etc.).
-- 3. Join with nodes to ensure the node is not drained.
-- 4. Use FOR UPDATE SKIP LOCKED to avoid contention.
-- 5. Update step status to 'assigned' and set node_id.
--
-- This ensures sequential execution: step k can only be claimed after step k-1 succeeds.
WITH eligible_steps AS (
  SELECT rs.id, rs.run_id, rs.step_index
  FROM run_steps rs
  INNER JOIN runs r ON r.id = rs.run_id
  INNER JOIN nodes n ON n.id = $1
  WHERE rs.status = 'queued'
    AND n.drained = false
    -- Step 0 can always be claimed; step k>0 requires step k-1 to have succeeded.
    AND (
      rs.step_index = 0
      OR EXISTS (
        SELECT 1 FROM run_steps prev
        WHERE prev.run_id = rs.run_id
          AND prev.step_index = rs.step_index - 1
          AND prev.status = 'succeeded'
      )
    )
  ORDER BY r.created_at, rs.step_index
  FOR UPDATE OF rs SKIP LOCKED
  LIMIT 1
)
UPDATE run_steps rs
SET status = 'assigned', node_id = $1, started_at = now()
FROM eligible_steps e
WHERE rs.id = e.id
RETURNING rs.*;

-- name: AckRunStepStart :exec
-- Acknowledges that a step has started execution (transitions from 'assigned' to 'running').
UPDATE run_steps
SET status = 'running'
WHERE id = $1 AND status = 'assigned';

-- name: UpdateRunStepCompletion :exec
-- Updates a step's terminal status (succeeded/failed/canceled) and timing.
UPDATE run_steps
SET status = $2, reason = $3, finished_at = now()
WHERE id = $1;

-- name: DeleteRunStep :exec
-- Deletes a step (usually for cleanup or test teardown).
DELETE FROM run_steps
WHERE id = $1;

-- name: CountRunSteps :one
-- Counts the total number of steps for a run.
SELECT COUNT(*) FROM run_steps
WHERE run_id = $1;

-- name: CountRunStepsByStatus :one
-- Counts steps for a run with a specific status.
SELECT COUNT(*) FROM run_steps
WHERE run_id = $1 AND status = $2;
