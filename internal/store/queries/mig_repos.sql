-- name: CreateMigRepo :one
INSERT INTO mig_repos (
  id,
  mig_id,
  repo_url,
  base_ref,
  target_ref,
  prep_status,
  prep_last_error,
  prep_failure_code,
  prep_profile,
  prep_artifacts,
  prep_updated_at
)
VALUES ($1, $2, $3, $4, $5, 'PrepPending', NULL, NULL, NULL, NULL, now())
RETURNING id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at;

-- name: GetMigRepo :one
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE id = $1;

-- name: GetMigRepoByURL :one
-- Gets a mod_repo by mig_id and repo_url (for uniqueness constraint enforcement).
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE mig_id = $1 AND repo_url = $2;

-- name: ListMigReposByMig :many
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE mig_id = $1
ORDER BY created_at ASC, id ASC;

-- name: UpdateMigRepoRefs :exec
UPDATE mig_repos
SET base_ref = $2,
    target_ref = $3
WHERE id = $1;

-- name: DeleteMigRepo :exec
-- Deletes a mod_repo by id.
-- Note: mig_repos.id is referenced by run_repos.repo_id and jobs.repo_id with ON DELETE RESTRICT.
-- This DELETE will fail if any run_repos/jobs rows still reference the repo.
DELETE FROM mig_repos
WHERE id = $1;

-- name: UpsertMigRepo :one
-- Bulk upsert a mod_repo by normalized repo_url.
-- Uniqueness is on (mig_id, repo_url) to prevent duplicate repo URLs per mig.
-- If a row exists, update refs; otherwise insert.
INSERT INTO mig_repos (
  id,
  mig_id,
  repo_url,
  base_ref,
  target_ref,
  prep_status,
  prep_last_error,
  prep_failure_code,
  prep_profile,
  prep_artifacts,
  prep_updated_at
)
VALUES ($1, $2, $3, $4, $5, 'PrepPending', NULL, NULL, NULL, NULL, now())
ON CONFLICT (mig_id, repo_url)
DO UPDATE SET
  base_ref = EXCLUDED.base_ref,
  target_ref = EXCLUDED.target_ref,
  prep_status = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN 'PrepPending'::prep_status
    ELSE mig_repos.prep_status
  END,
  prep_last_error = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN NULL
    ELSE mig_repos.prep_last_error
  END,
  prep_failure_code = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN NULL
    ELSE mig_repos.prep_failure_code
  END,
  prep_profile = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN NULL
    ELSE mig_repos.prep_profile
  END,
  prep_artifacts = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN NULL
    ELSE mig_repos.prep_artifacts
  END,
  prep_updated_at = CASE
    WHEN mig_repos.base_ref IS DISTINCT FROM EXCLUDED.base_ref
      OR mig_repos.target_ref IS DISTINCT FROM EXCLUDED.target_ref
    THEN now()
    ELSE mig_repos.prep_updated_at
  END
RETURNING id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at;

-- name: ListReposByPrepStatus :many
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_status, prep_attempts, prep_last_error, prep_failure_code, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE prep_status = $1
ORDER BY prep_updated_at ASC, created_at ASC, id ASC;

-- name: ClaimNextPrepRepo :one
WITH candidate AS (
  SELECT mr.id
  FROM mig_repos mr
  WHERE mr.prep_status = 'PrepPending'
  ORDER BY mr.prep_updated_at ASC, mr.created_at ASC, mr.id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE mig_repos mr
SET prep_status = 'PrepRunning',
    prep_attempts = mr.prep_attempts + 1,
    prep_last_error = NULL,
    prep_failure_code = NULL,
    prep_updated_at = now()
FROM candidate
WHERE mr.id = candidate.id
  AND mr.prep_status = 'PrepPending'
RETURNING mr.id, mr.mig_id, mr.repo_url, mr.base_ref, mr.target_ref, mr.prep_status, mr.prep_attempts, mr.prep_last_error, mr.prep_failure_code, mr.prep_updated_at, mr.prep_profile, mr.prep_artifacts, mr.created_at;

-- name: ClaimNextPrepRetryRepo :one
WITH candidate AS (
  SELECT mr.id
  FROM mig_repos mr
  WHERE mr.prep_status = 'PrepRetryScheduled'
    AND mr.prep_updated_at <= $1
  ORDER BY mr.prep_updated_at ASC, mr.created_at ASC, mr.id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE mig_repos mr
SET prep_status = 'PrepRunning',
    prep_attempts = mr.prep_attempts + 1,
    prep_last_error = NULL,
    prep_failure_code = NULL,
    prep_updated_at = now()
FROM candidate
WHERE mr.id = candidate.id
  AND mr.prep_status = 'PrepRetryScheduled'
RETURNING mr.id, mr.mig_id, mr.repo_url, mr.base_ref, mr.target_ref, mr.prep_status, mr.prep_attempts, mr.prep_last_error, mr.prep_failure_code, mr.prep_updated_at, mr.prep_profile, mr.prep_artifacts, mr.created_at;

-- name: UpdateMigRepoPrepState :exec
UPDATE mig_repos
SET prep_status = $2,
    prep_last_error = $3,
    prep_failure_code = $4,
    prep_updated_at = now()
WHERE id = $1;

-- name: SaveMigRepoPrepProfile :exec
UPDATE mig_repos
SET prep_profile = $2,
    prep_artifacts = $3,
    prep_status = 'PrepReady',
    prep_last_error = NULL,
    prep_failure_code = NULL,
    prep_updated_at = now()
WHERE id = $1;

-- name: HasMigRepoHistory :one
-- Checks if a mod_repo has any historical executions (run_repos references).
-- Returns true if the repo cannot be deleted due to history, false otherwise.
-- Deletion is refused if the repo has historical executions.
SELECT EXISTS(
  SELECT 1 FROM run_repos WHERE repo_id = $1 LIMIT 1
) AS has_history;

-- name: ListDistinctRepos :many
-- v1: Lists distinct repos (mig_repos) with last known run metadata, optionally filtered by repo_url substring.
SELECT
  mr.id AS repo_id,
  mr.repo_url,
  rr.last_run_at,
  COALESCE(rr.last_status::text, '') AS last_status
FROM mig_repos mr
LEFT JOIN LATERAL (
  SELECT
    rrr.started_at AS last_run_at,
    rrr.status AS last_status
  FROM run_repos rrr
  WHERE rrr.repo_id = mr.id
  ORDER BY rrr.started_at DESC NULLS LAST, rrr.created_at DESC, rrr.run_id DESC
  LIMIT 1
) rr ON true
WHERE (@filter::text IS NULL OR @filter = '' OR mr.repo_url ILIKE '%' || @filter || '%')
ORDER BY mr.repo_url ASC, mr.id ASC;
