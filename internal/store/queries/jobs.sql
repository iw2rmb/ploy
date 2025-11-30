-- name: GetJob :one
SELECT * FROM jobs
WHERE id = $1;

-- name: ListJobsByRun :many
SELECT * FROM jobs
WHERE run_id = $1
ORDER BY step_index ASC;

-- name: CreateJob :one
INSERT INTO jobs (run_id, name, status, step_index, meta)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2, started_at = $3, finished_at = $4, duration_ms = $5
WHERE id = $1;

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: ClaimJob :one
-- Atomically claim the next scheduled job for a node.
-- Server-driven scheduling: only 'scheduled' jobs are claimable.
-- Job transitions directly to 'running' (no intermediate 'assigned' state).
WITH eligible AS (
  SELECT j.id FROM jobs j
  WHERE j.run_id IN (SELECT id FROM runs WHERE status IN ('queued', 'running'))
    AND j.status = 'scheduled'
    AND j.node_id IS NULL
  ORDER BY j.step_index ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE jobs SET status = 'running', node_id = $1, started_at = now()
FROM eligible WHERE jobs.id = eligible.id
RETURNING jobs.*;

-- name: GetAdjacentJobIndices :one
-- Get the step_index of a job and the next job's step_index for healing insertion.
-- Returns prev_index (the given job's index) and next_index (the following job's index, or NULL if none).
SELECT
  j1.step_index as prev_index,
  (SELECT MIN(j2.step_index) FROM jobs j2 WHERE j2.run_id = j1.run_id AND j2.step_index > j1.step_index) as next_index
FROM jobs j1 WHERE j1.id = $1;

-- name: ListCreatedJobsByRun :many
-- List all created (not yet scheduled) jobs for a run, ordered by step_index.
SELECT * FROM jobs
WHERE run_id = $1 AND status = 'created'
ORDER BY step_index ASC;

-- name: ScheduleNextJob :one
-- Transitions the first 'created' job to 'scheduled' for a run.
-- Called by server after a job completes successfully to enable server-driven scheduling.
-- Returns the scheduled job, or null if no more jobs to schedule.
UPDATE jobs SET status = 'scheduled'
WHERE id = (
  SELECT j.id FROM jobs j
  WHERE j.run_id = $1 AND j.status = 'created'
  ORDER BY j.step_index ASC
  LIMIT 1
)
RETURNING *;

-- name: GetJobByRunAndStepIndex :one
-- Retrieves a specific job by run_id and step_index.
SELECT * FROM jobs
WHERE run_id = $1 AND step_index = $2;

-- name: CountJobsByRun :one
-- Counts total jobs for a run.
SELECT COUNT(*) FROM jobs
WHERE run_id = $1;

-- name: CountJobsByRunAndStatus :one
-- Counts jobs for a run with a specific status.
SELECT COUNT(*) FROM jobs
WHERE run_id = $1 AND status = $2;

-- name: UpdateJobCompletion :exec
-- Updates a job's terminal status, exit code, and timing.
UPDATE jobs
SET status = $2, exit_code = $3, finished_at = now(),
    duration_ms = EXTRACT(EPOCH FROM (now() - started_at)) * 1000
WHERE id = $1;
