-- name: CreateModRepo :one
INSERT INTO mod_repos (id, mod_id, repo_url, base_ref, target_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, mod_id, repo_url, base_ref, target_ref, created_at;

-- name: GetModRepo :one
SELECT id, mod_id, repo_url, base_ref, target_ref, created_at
FROM mod_repos
WHERE id = $1;

-- name: ListModReposByMod :many
SELECT id, mod_id, repo_url, base_ref, target_ref, created_at
FROM mod_repos
WHERE mod_id = $1
ORDER BY created_at ASC;

-- name: UpdateModRepoRefs :exec
UPDATE mod_repos
SET base_ref = $2,
    target_ref = $3
WHERE id = $1;

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
ORDER BY mr.repo_url ASC;
