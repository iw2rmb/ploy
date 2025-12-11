-- name: GetJob :one
SELECT id, run_id, name, status, mod_type, mod_image, step_index, node_id, exit_code, started_at, finished_at, duration_ms, meta
FROM jobs
WHERE id = $1;

-- name: ListJobsByRun :many
SELECT id, run_id, name, status, mod_type, mod_image, step_index, node_id, exit_code, started_at, finished_at, duration_ms, meta
FROM jobs
WHERE run_id = $1
ORDER BY step_index ASC;

-- name: CreateJob :one
-- Note: `id` is now a required TEXT parameter (KSUID-backed); caller generates via types.NewJobID().
INSERT INTO jobs (id, run_id, name, status, mod_type, mod_image, step_index, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, run_id, name, status, mod_type, mod_image, step_index, node_id, exit_code, started_at, finished_at, duration_ms, meta;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = $2, started_at = $3, finished_at = $4, duration_ms = $5
WHERE id = $1;

-- name: DeleteJob :exec
DELETE FROM jobs
WHERE id = $1;

-- name: ClaimJob :one
-- Atomically claim the next pending job for a node (single unified queue).
-- Jobs are ordered by step_index; no special handling for gate vs mod jobs.
-- Server-driven scheduling: only 'pending' jobs are claimable.
-- Job transitions directly to 'running' (no intermediate 'assigned' state).
WITH eligible AS (
  SELECT j.id FROM jobs j
  JOIN runs r ON j.run_id = r.id
  WHERE j.status = 'pending'
    AND j.node_id IS NULL
    AND (
      -- Normal jobs: only when run is queued or running.
      r.status IN ('queued', 'running') OR
      -- MR jobs: allowed after run has reached a terminal state.
      (j.mod_type = 'mr' AND r.status IN ('succeeded', 'failed'))
    )
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
-- List all created (not yet pending) jobs for a run, ordered by step_index.
SELECT id, run_id, name, status, mod_type, mod_image, step_index, node_id, exit_code, started_at, finished_at, duration_ms, meta
FROM jobs
WHERE run_id = $1 AND status = 'created'
ORDER BY step_index ASC;

-- name: ScheduleNextJob :one
-- Transitions the first 'created' job to 'pending' for a run.
-- Called by server after a job completes successfully to enable server-driven scheduling.
-- Returns the pending job, or null if no more jobs to schedule.
UPDATE jobs SET status = 'pending'
WHERE id = (
  SELECT j.id FROM jobs j
  WHERE j.run_id = $1 AND j.status = 'created'
  ORDER BY j.step_index ASC
  LIMIT 1
)
RETURNING id, run_id, name, status, mod_type, mod_image, step_index, node_id, exit_code, started_at, finished_at, duration_ms, meta;

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

-- name: UpdateJobMeta :exec
-- Updates a job's meta JSONB field with structured gate/build metadata.
-- Used to persist gate validation results or build metrics after job execution.
-- The meta parameter should be JSON-encoded JobMeta (see internal/workflow/contracts.JobMeta).
UPDATE jobs
SET meta = $2
WHERE id = $1;

-- name: UpdateJobCompletionWithMeta :exec
-- Updates a job's terminal status, exit code, timing, and meta in one operation.
-- Use this when completing a gate or build job that has execution metadata.
UPDATE jobs
SET status = $2, exit_code = $3, finished_at = now(),
    duration_ms = EXTRACT(EPOCH FROM (now() - started_at)) * 1000,
    meta = $4
WHERE id = $1;
