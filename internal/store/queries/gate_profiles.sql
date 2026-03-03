-- name: ResolveStackIDByImage :one
SELECT id
FROM stacks
WHERE image = sqlc.arg(image)
ORDER BY id ASC
LIMIT 1;

-- name: ResolveStackIDByRequiredStack :one
SELECT id
FROM stacks
WHERE lang = sqlc.arg(lang)::text
  AND release = sqlc.arg(release)::text
  AND (sqlc.arg(tool)::text = '' OR COALESCE(tool, '') = sqlc.arg(tool)::text)
ORDER BY id ASC
LIMIT 1;

-- name: ResolveStackRowByImage :one
SELECT id,
       lang,
       COALESCE(tool, '') AS tool,
       release
FROM stacks
WHERE image = sqlc.arg(image)
ORDER BY id ASC
LIMIT 1;

-- name: ResolveStackRowByLangToolRelease :one
SELECT id,
       lang,
       COALESCE(tool, '') AS tool,
       release
FROM stacks
WHERE lang = sqlc.arg(lang)::text
  AND COALESCE(tool, '') = sqlc.arg(tool)::text
  AND release = sqlc.arg(release)::text
ORDER BY id ASC
LIMIT 1;

-- name: ResolveStackRowByLangTool :one
SELECT id,
       lang,
       COALESCE(tool, '') AS tool,
       release
FROM stacks
WHERE lang = sqlc.arg(lang)::text
  AND COALESCE(tool, '') = sqlc.arg(tool)::text
ORDER BY id ASC
LIMIT 1;

-- name: ResolveStackIDByRepoSHA :one
SELECT stack_id
FROM gate_profiles
WHERE COALESCE(repo_id, '') = sqlc.arg(repo_id)::text
  AND COALESCE(repo_sha, '') = sqlc.arg(repo_sha)::text
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: ResolveAnyStackID :one
SELECT id
FROM stacks
ORDER BY id ASC
LIMIT 1;

-- name: GetExactGateProfile :one
SELECT id,
       COALESCE(repo_id, '') AS repo_id,
       COALESCE(repo_sha, '') AS repo_sha,
       COALESCE(repo_sha8, '') AS repo_sha8,
       stack_id,
       url
FROM gate_profiles
WHERE COALESCE(repo_id, '') = sqlc.arg(repo_id)::text
  AND COALESCE(repo_sha, '') = sqlc.arg(repo_sha)::text
  AND stack_id = sqlc.arg(stack_id)
LIMIT 1;

-- name: GetLatestRepoGateProfile :one
SELECT id,
       COALESCE(repo_id, '') AS repo_id,
       COALESCE(repo_sha, '') AS repo_sha,
       COALESCE(repo_sha8, '') AS repo_sha8,
       stack_id,
       url
FROM gate_profiles
WHERE COALESCE(repo_id, '') = sqlc.arg(repo_id)::text
  AND stack_id = sqlc.arg(stack_id)
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: GetDefaultGateProfile :one
SELECT id,
       COALESCE(repo_id, '') AS repo_id,
       COALESCE(repo_sha, '') AS repo_sha,
       COALESCE(repo_sha8, '') AS repo_sha8,
       stack_id,
       url
FROM gate_profiles
WHERE repo_id IS NULL
  AND repo_sha IS NULL
  AND stack_id = sqlc.arg(stack_id)
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: UpsertExactGateProfile :one
INSERT INTO gate_profiles (
  repo_id,
  repo_sha,
  repo_sha8,
  stack_id,
  url
)
VALUES (
  sqlc.arg(repo_id)::text,
  sqlc.arg(repo_sha)::text,
  SUBSTRING(sqlc.arg(repo_sha)::text, 1, 8),
  sqlc.arg(stack_id),
  sqlc.arg(url)
)
ON CONFLICT (repo_id, repo_sha, stack_id)
DO UPDATE SET
  url = EXCLUDED.url,
  repo_sha8 = EXCLUDED.repo_sha8,
  updated_at = NOW()
RETURNING id,
          COALESCE(repo_id, '') AS repo_id,
          COALESCE(repo_sha, '') AS repo_sha,
          COALESCE(repo_sha8, '') AS repo_sha8,
          stack_id,
          url;

-- name: ResolvePreGateCreationBindingByRepoSHAAndStack :one
SELECT gp.id AS profile_id,
       COALESCE(s.image, '') AS job_image
FROM gate_profiles gp
JOIN stacks s ON s.id = gp.stack_id
WHERE COALESCE(gp.repo_id, '') = sqlc.arg(repo_id)::text
  AND COALESCE(gp.repo_sha, '') = sqlc.arg(repo_sha)::text
  AND (sqlc.arg(lang)::text = '' OR s.lang = sqlc.arg(lang)::text)
  AND (sqlc.arg(tool)::text = '' OR COALESCE(s.tool, '') = sqlc.arg(tool)::text)
  AND (sqlc.arg(release)::text = '' OR s.release = sqlc.arg(release)::text)
ORDER BY gp.updated_at DESC, gp.id DESC
LIMIT 1;

-- name: ResolvePreGateCreationBindingByRepoSHA :one
SELECT gp.id AS profile_id,
       COALESCE(s.image, '') AS job_image
FROM gate_profiles gp
JOIN stacks s ON s.id = gp.stack_id
WHERE COALESCE(gp.repo_id, '') = sqlc.arg(repo_id)::text
  AND COALESCE(gp.repo_sha, '') = sqlc.arg(repo_sha)::text
ORDER BY gp.updated_at DESC, gp.id DESC
LIMIT 1;

-- name: UpsertGateJobProfileLink :exec
INSERT INTO gates (job_id, profile_id)
VALUES (sqlc.arg(job_id), sqlc.arg(profile_id))
ON CONFLICT (job_id)
DO UPDATE SET profile_id = EXCLUDED.profile_id;
