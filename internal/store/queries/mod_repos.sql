-- name: CreateModRepo :one
INSERT INTO mod_repos (id, mod_id, repo_url, base_ref, target_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, mod_id, repo_url, base_ref, target_ref, created_at;

-- name: GetModRepo :one
SELECT id, mod_id, repo_url, base_ref, target_ref, created_at
FROM mod_repos
WHERE id = $1;

-- name: GetModRepoByURL :one
-- Gets a mod_repo by mod_id and repo_url (for uniqueness constraint enforcement).
SELECT id, mod_id, repo_url, base_ref, target_ref, created_at
FROM mod_repos
WHERE mod_id = $1 AND repo_url = $2;

-- name: ListModReposByMod :many
SELECT id, mod_id, repo_url, base_ref, target_ref, created_at
FROM mod_repos
WHERE mod_id = $1
ORDER BY created_at ASC, id ASC;

-- name: UpdateModRepoRefs :exec
UPDATE mod_repos
SET base_ref = $2,
    target_ref = $3
WHERE id = $1;

-- name: DeleteModRepo :exec
-- Deletes a mod_repo by id.
-- Note: mod_repos.id is referenced by run_repos.repo_id and jobs.repo_id with ON DELETE RESTRICT.
-- This DELETE will fail if any run_repos/jobs rows still reference the repo.
DELETE FROM mod_repos
WHERE id = $1;

-- name: UpsertModRepo :one
-- Bulk upsert a mod_repo by normalized repo_url.
-- Uniqueness is on (mod_id, repo_url) to prevent duplicate repo URLs per mod.
-- If a row exists, update refs; otherwise insert.
INSERT INTO mod_repos (id, mod_id, repo_url, base_ref, target_ref)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (mod_id, repo_url)
DO UPDATE SET
  base_ref = EXCLUDED.base_ref,
  target_ref = EXCLUDED.target_ref
RETURNING id, mod_id, repo_url, base_ref, target_ref, created_at;

-- name: HasModRepoHistory :one
-- Checks if a mod_repo has any historical executions (run_repos references).
-- Returns true if the repo cannot be deleted due to history, false otherwise.
-- Deletion is refused if the repo has historical executions.
SELECT EXISTS(
  SELECT 1 FROM run_repos WHERE repo_id = $1 LIMIT 1
) AS has_history;

-- name: ListDistinctRepos :many
-- v1: Lists distinct repos (mod_repos) with last known run metadata, optionally filtered by repo_url substring.
SELECT
  mr.id AS repo_id,
  mr.repo_url,
  rr.last_run_at,
  COALESCE(rr.last_status::text, '') AS last_status
FROM mod_repos mr
LEFT JOIN LATERAL (
  SELECT
    rrr.started_at AS last_run_at,
    rrr.status AS last_status
  FROM run_repos rrr
  WHERE rrr.repo_id = mr.id
  ORDER BY rrr.started_at DESC NULLS LAST, rrr.created_at DESC
  LIMIT 1
) rr ON true
WHERE (@filter::text IS NULL OR @filter = '' OR mr.repo_url ILIKE '%' || @filter || '%')
ORDER BY mr.repo_url ASC, mr.id ASC;
