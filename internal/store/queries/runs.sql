-- name: GetRun :one
SELECT id, mod_id, spec_id, created_by, status, created_at, started_at, finished_at, stats
FROM runs
WHERE id = $1;

-- name: ListRuns :many
SELECT id, mod_id, spec_id, created_by, status, created_at, started_at, finished_at, stats
FROM runs
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CreateRun :one
-- v1: Creates a new run for a mod + spec snapshot. Runs are created in Started state.
-- Note: `id` is a required TEXT parameter (KSUID-backed); caller generates via types.NewRunID().
INSERT INTO runs (id, mod_id, spec_id, created_by, status, started_at)
VALUES ($1, $2, $3, $4, 'Started', now())
RETURNING id, mod_id, spec_id, created_by, status, created_at, started_at, finished_at, stats;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2,
    finished_at = CASE
      WHEN $2 IN ('Cancelled'::run_status, 'Finished'::run_status) THEN COALESCE(finished_at, now())
      ELSE NULL
    END
WHERE id = $1;

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

-- name: UpdateRunStatsMRURL :exec
-- Merge an MR URL into runs.stats.metadata.mr_url without altering other fields.
-- Preserves existing stats and metadata keys via JSONB merge.
	UPDATE runs
	SET stats = COALESCE(stats, '{}'::jsonb) || jsonb_build_object(
	    'metadata',
	    COALESCE(stats->'metadata', '{}'::jsonb) || jsonb_build_object('mr_url', sqlc.arg(mr_url))
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
