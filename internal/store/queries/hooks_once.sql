-- name: GetHookOnceLedger :one
SELECT
  run_id,
  repo_id,
  hook_hash,
  first_success_job_id,
  last_success_job_id,
  last_skip_job_id,
  once_skip_marked,
  created_at,
  updated_at
FROM hooks_once
WHERE run_id = $1
  AND repo_id = $2
  AND hook_hash = $3;

-- name: HasHookOnceLedger :one
SELECT EXISTS (
  SELECT 1
  FROM hooks_once
  WHERE run_id = $1
    AND repo_id = $2
    AND hook_hash = $3
);

-- name: ListHookOnceLedgerByRunRepo :many
SELECT
  run_id,
  repo_id,
  hook_hash,
  first_success_job_id,
  last_success_job_id,
  last_skip_job_id,
  once_skip_marked,
  created_at,
  updated_at
FROM hooks_once
WHERE run_id = $1
  AND repo_id = $2
ORDER BY hook_hash ASC;

-- name: UpsertHookOnceSuccess :exec
INSERT INTO hooks_once (
  run_id,
  repo_id,
  hook_hash,
  first_success_job_id,
  last_success_job_id
) VALUES (
  $1, $2, $3, $4, $4
)
ON CONFLICT (run_id, repo_id, hook_hash)
DO UPDATE
SET last_success_job_id = EXCLUDED.last_success_job_id,
    updated_at = now();

-- name: MarkHookOnceSkipped :exec
INSERT INTO hooks_once (
  run_id,
  repo_id,
  hook_hash,
  last_skip_job_id,
  once_skip_marked
) VALUES (
  $1, $2, $3, $4, true
)
ON CONFLICT (run_id, repo_id, hook_hash)
DO UPDATE
SET once_skip_marked = true,
    last_skip_job_id = EXCLUDED.last_skip_job_id,
    updated_at = now();
