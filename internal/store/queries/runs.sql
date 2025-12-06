-- name: GetRun :one
SELECT * FROM runs
WHERE id = $1;

-- name: ListRuns :many
SELECT * FROM runs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateRun :one
-- Creates a new run record. The `name` column is optional; pass NULL for unnamed runs.
INSERT INTO runs (name, repo_url, spec, created_by, status, base_ref, target_ref, commit_sha)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2, finished_at = $3
WHERE id = $1;

-- name: AckRunStart :exec
-- Transitions run status to 'running' when execution starts.
-- Jobs are claimed via ClaimJob in jobs.sql; runs transition to running when
-- the first job starts execution.
UPDATE runs
SET status = 'running'
WHERE id = $1 AND status IN ('assigned', 'queued');

-- name: UpdateRunCompletion :exec
UPDATE runs
SET status = $2, finished_at = now(), stats = $3
WHERE id = $1;

-- name: DeleteRun :exec
DELETE FROM runs
WHERE id = $1;

-- name: UpdateRunResume :exec
-- Increments resume_count and updates last_resumed_at timestamp in runs.stats.
-- Uses JSONB merge (||) to preserve existing stats while adding resume metadata.
UPDATE runs
SET stats = stats || jsonb_build_object(
    'resume_count', COALESCE((stats->>'resume_count')::int, 0) + 1,
    'last_resumed_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
)
WHERE id = $1;

-- name: GetRunTiming :one
SELECT id,
       COALESCE(queue_ms, 0) AS queue_ms,
       COALESCE(run_ms, 0)   AS run_ms
FROM runs_timing
WHERE id = $1;

-- name: ListRunsTimings :many
SELECT id,
       COALESCE(queue_ms, 0) AS queue_ms,
       COALESCE(run_ms, 0)   AS run_ms
FROM runs_timing
ORDER BY id DESC
LIMIT $1 OFFSET $2;
