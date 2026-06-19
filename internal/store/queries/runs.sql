-- name: GetRun :one
SELECT id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
       created_by, status, attempt, last_error, created_at, started_at, finished_at, stats
FROM runs
WHERE id = $1;

-- name: ListRuns :many
SELECT id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
       created_by, status, attempt, last_error, created_at, started_at, finished_at, stats
FROM runs
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ListRunsWithMetadata :many
SELECT
  runs.id,
  runs.wave_id,
  runs.mig_id,
  runs.spec_id,
  runs.repo_id,
  runs.repo_base_ref,
  runs.source_commit_sha,
  runs.repo_sha0,
  runs.created_by,
  runs.status,
  runs.attempt,
  runs.last_error,
  runs.created_at,
  runs.started_at,
  runs.finished_at,
  runs.stats,
  repos.url AS repo_url,
  specs.name AS spec_name,
  COALESCE(specs.source->>'domain', '')::text AS spec_source_domain,
  COALESCE(specs.source->>'repo', '')::text AS spec_source_repo
FROM runs
JOIN repos ON repos.id = runs.repo_id
JOIN specs ON specs.id = runs.spec_id
WHERE (sqlc.arg(all_runs)::boolean OR runs.created_by = sqlc.arg(created_by)::text)
  AND (
    sqlc.arg(repo_url)::text = ''
    OR repos.url = sqlc.arg(repo_url)::text
  )
ORDER BY runs.created_at DESC, runs.id DESC
LIMIT sqlc.arg(limit_rows)::int OFFSET sqlc.arg(offset_rows)::int;

-- name: ListRunsByWave :many
SELECT id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
       created_by, status, attempt, last_error, created_at, started_at, finished_at, stats
FROM runs
WHERE wave_id = $1
ORDER BY created_at ASC, id ASC;

-- name: ListRunsWithURLByWave :many
SELECT runs.id, runs.wave_id, runs.mig_id, runs.spec_id, runs.repo_id, runs.repo_base_ref,
       runs.source_commit_sha, runs.repo_sha0, runs.created_by, runs.status, runs.attempt,
       runs.last_error, runs.created_at, runs.started_at, runs.finished_at, runs.stats,
       repos.url AS repo_url
FROM runs
JOIN repos ON repos.id = runs.repo_id
WHERE runs.wave_id = $1
ORDER BY runs.created_at ASC, runs.id ASC;

-- name: ListRunsForRepo :many
SELECT
  runs.id AS run_id,
  runs.wave_id,
  runs.mig_id,
  runs.status,
  runs.repo_base_ref,
  runs.attempt,
  runs.started_at,
  runs.finished_at
FROM runs
WHERE runs.repo_id = $1
ORDER BY runs.created_at DESC, runs.id DESC
LIMIT $2 OFFSET $3;

-- name: CreateRun :one
INSERT INTO runs (
  id,
  wave_id,
  mig_id,
  spec_id,
  repo_id,
  repo_base_ref,
  source_commit_sha,
  repo_sha0,
  created_by,
  status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'Queued')
RETURNING id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
          created_by, status, attempt, last_error, created_at, started_at, finished_at, stats;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2,
    started_at = CASE WHEN $2 = 'Running'::run_status AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN $2 IN ('Success'::run_status, 'Fail'::run_status, 'Cancelled'::run_status) THEN COALESCE(finished_at, now()) ELSE finished_at END
WHERE id = $1;

-- name: UpdateRunError :exec
UPDATE runs
SET last_error = $2
WHERE id = $1;

-- name: IncrementRunAttempt :exec
UPDATE runs
SET attempt = attempt + 1,
    status = 'Queued',
    last_error = NULL,
    started_at = NULL,
    finished_at = NULL,
    stats = '{}'::jsonb
WHERE id = $1;

-- name: UpdateRunBaseRef :exec
UPDATE runs
SET repo_base_ref = $2
WHERE id = $1;

-- name: CountRunsByWaveStatus :many
SELECT status, COUNT(*)::int AS count
FROM runs
WHERE wave_id = $1
GROUP BY status;

-- name: CancelActiveRunsByWave :execrows
UPDATE runs
SET status = 'Cancelled',
    finished_at = COALESCE(finished_at, now())
WHERE wave_id = $1
  AND status IN ('Queued', 'Running');

-- name: DeleteRun :exec
DELETE FROM runs
WHERE id = $1;

-- name: ListQueuedRunsByWave :many
SELECT id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
       created_by, status, attempt, last_error, created_at, started_at, finished_at, stats
FROM runs
WHERE wave_id = $1
  AND status = 'Queued'
ORDER BY created_at ASC, id ASC;

-- name: ListWavesWithQueuedRuns :many
SELECT DISTINCT wave_id
FROM runs
JOIN waves ON waves.id = runs.wave_id
WHERE waves.status = 'Started'
  AND runs.status = 'Queued'
ORDER BY wave_id;

-- name: ListFailedRepoIDsByMig :many
SELECT repo_id FROM (
  SELECT DISTINCT ON (repo_id) repo_id, status
  FROM runs
  WHERE mig_id = $1
    AND status IN ('Fail', 'Success', 'Cancelled')
  ORDER BY repo_id, created_at DESC, id DESC
) AS last_status
WHERE status = 'Fail';

-- name: GetRunSnapshotMetadata :one
SELECT
  runs.id AS run_id,
  runs.repo_id,
  runs.repo_base_ref,
  runs.source_commit_sha,
  repos.url AS repo_url
FROM runs
JOIN repos ON repos.id = runs.repo_id
WHERE runs.id = $1;

-- name: HasRunningJobForRunNode :one
SELECT EXISTS (
  SELECT 1
  FROM jobs
  WHERE run_id = $1
    AND node_id = $2
    AND status = 'Running'
)::boolean;

-- name: GetLatestRunByMigAndRepoStatus :one
SELECT id AS run_id, repo_id
FROM runs
WHERE mig_id = $1
  AND repo_id = $2
  AND status = $3
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateRunResume :exec
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
