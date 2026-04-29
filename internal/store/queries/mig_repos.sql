-- name: CreateMigRepo :one
WITH existing_repo AS (
  SELECT id
  FROM repos
  WHERE repos.url = $3
),
inserted_repo AS (
  INSERT INTO repos (id, url)
  SELECT $1, $3
  WHERE NOT EXISTS (SELECT 1 FROM existing_repo)
  ON CONFLICT (url) DO NOTHING
  RETURNING id
),
resolved_repo AS (
  SELECT id FROM inserted_repo
  UNION ALL
  SELECT id FROM existing_repo
  UNION ALL
  SELECT id FROM repos WHERE repos.url = $3
  LIMIT 1
)
INSERT INTO mig_repos (
  id,
  mig_id,
  repo_id,
  base_ref,
  target_ref
)
VALUES ($1, $2, (SELECT id FROM resolved_repo), $4, $5)
RETURNING id, mig_id, repo_id, base_ref, target_ref, created_at;

-- name: GetMigRepo :one
SELECT id, mig_id, repo_id, base_ref, target_ref, created_at
FROM mig_repos
WHERE id = $1;

-- name: GetMigRepoByURL :one
-- Gets a mig_repo by mig_id and repo_url (for uniqueness constraint enforcement).
SELECT mr.id, mr.mig_id, mr.repo_id, mr.base_ref, mr.target_ref, mr.created_at
FROM mig_repos mr
JOIN repos r ON r.id = mr.repo_id
WHERE mr.mig_id = $1 AND r.url = $2;

-- name: ListMigReposByMig :many
SELECT id, mig_id, repo_id, base_ref, target_ref, created_at
FROM mig_repos
WHERE mig_id = $1
ORDER BY created_at ASC, id ASC;

-- name: UpdateMigRepoRefs :exec
UPDATE mig_repos
SET base_ref = $2,
    target_ref = $3
WHERE id = $1;

-- name: DeleteMigRepo :exec
-- Deletes a mig_repo by id.
-- Note: mig_repos.id remains referenced by API-level repo membership records.
DELETE FROM mig_repos
WHERE id = $1;

-- name: UpsertMigRepo :one
-- Bulk upsert a mig_repo by normalized repo_url.
-- Uniqueness is on (mig_id, repo_id) to prevent duplicate repo membership per mig.
WITH existing_repo AS (
  SELECT id
  FROM repos
  WHERE repos.url = $3
),
inserted_repo AS (
  INSERT INTO repos (id, url)
  SELECT $1, $3
  WHERE NOT EXISTS (SELECT 1 FROM existing_repo)
  ON CONFLICT (url) DO NOTHING
  RETURNING id
),
resolved_repo AS (
  SELECT id FROM inserted_repo
  UNION ALL
  SELECT id FROM existing_repo
  UNION ALL
  SELECT id FROM repos WHERE repos.url = $3
  LIMIT 1
)
INSERT INTO mig_repos (
  id,
  mig_id,
  repo_id,
  base_ref,
  target_ref
)
VALUES ($1, $2, (SELECT id FROM resolved_repo), $4, $5)
ON CONFLICT (mig_id, repo_id)
DO UPDATE SET
  base_ref = EXCLUDED.base_ref,
  target_ref = EXCLUDED.target_ref
RETURNING id, mig_id, repo_id, base_ref, target_ref, created_at;

-- name: HasMigRepoHistory :one
-- Checks if a mig_repo has any historical executions (run_repos references).
-- Returns true if the repo cannot be deleted due to history, false otherwise.
SELECT EXISTS(
  SELECT 1 FROM run_repos WHERE repo_id = $1 LIMIT 1
) AS has_history;

-- name: ListDistinctRepos :many
-- Lists distinct repos for a mig with last known run metadata,
-- optionally filtered by repo_url substring.
SELECT
  mr.repo_id,
  r.url AS repo_url,
  rr.last_run_at,
  COALESCE(rr.last_status::text, '') AS last_status
FROM mig_repos mr
JOIN repos r ON r.id = mr.repo_id
LEFT JOIN LATERAL (
  SELECT
    rrr.started_at AS last_run_at,
    rrr.status AS last_status
  FROM run_repos rrr
  WHERE rrr.repo_id = mr.repo_id
    AND rrr.mig_id = mr.mig_id
  ORDER BY rrr.started_at DESC NULLS LAST, rrr.created_at DESC, rrr.run_id DESC
  LIMIT 1
) rr ON true
WHERE (@filter::text IS NULL OR @filter = '' OR r.url ILIKE '%' || @filter || '%')
ORDER BY r.url ASC, mr.repo_id ASC;
