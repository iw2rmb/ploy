-- name: CreateMigRepo :one
INSERT INTO mig_repos (
  id,
  mig_id,
  repo_url,
  base_ref,
  target_ref,
  prep_profile,
  prep_artifacts,
  prep_updated_at
)
VALUES ($1, $2, $3, $4, $5, NULL, NULL, now())
RETURNING id, mig_id, repo_url, base_ref, target_ref, prep_updated_at, prep_profile, prep_artifacts, created_at;

-- name: GetMigRepo :one
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE id = $1;

-- name: GetMigRepoByURL :one
-- Gets a mod_repo by mig_id and repo_url (for uniqueness constraint enforcement).
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_updated_at, prep_profile, prep_artifacts, created_at
FROM mig_repos
WHERE mig_id = $1 AND repo_url = $2;

-- name: ListMigReposByMig :many
SELECT id, mig_id, repo_url, base_ref, target_ref, prep_updated_at, prep_profile, prep_artifacts, created_at
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
  prep_profile,
  prep_artifacts,
  prep_updated_at
)
VALUES ($1, $2, $3, $4, $5, NULL, NULL, now())
ON CONFLICT (mig_id, repo_url)
DO UPDATE SET
  base_ref = EXCLUDED.base_ref,
  target_ref = EXCLUDED.target_ref
RETURNING id, mig_id, repo_url, base_ref, target_ref, prep_updated_at, prep_profile, prep_artifacts, created_at;

-- name: PromoteReGateRecoveryCandidatePrepProfile :one
WITH target_job AS (
  SELECT j.id, j.repo_id
  FROM jobs j
  WHERE j.id = $1
    AND j.job_type = 're_gate'
    AND j.status = 'Success'
    AND (
      CASE
        WHEN jsonb_typeof(j.meta->'recovery'->'candidate_promoted') = 'boolean'
        THEN (j.meta->'recovery'->>'candidate_promoted')::boolean
        ELSE false
      END
    ) = false
  FOR UPDATE
),
updated_job AS (
  UPDATE jobs j
  SET meta = $2
  FROM target_job tj
  WHERE j.id = tj.id
  RETURNING tj.repo_id
)
UPDATE mig_repos mr
SET prep_profile = $3,
    prep_artifacts = $4,
    prep_updated_at = now()
FROM updated_job uj
WHERE mr.id = uj.repo_id
RETURNING mr.id;

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
