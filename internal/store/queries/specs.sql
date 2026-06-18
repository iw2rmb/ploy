-- name: CreateSpec :one
INSERT INTO specs (id, name, spec, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at;

-- name: CreateNamedSpec :one
INSERT INTO specs (id, name, description, source, sha, source_committed_at, spec, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at;

-- name: GetSpec :one
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM specs
WHERE id = $1;

-- name: GetNamedSpecByNameSourceSHA :one
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM specs
WHERE name = $1
  AND source->>'domain' = sqlc.arg(domain)::text
  AND source->>'repo' = sqlc.arg(repo)::text
  AND sha = sqlc.arg(sha)::text
  AND sha <> '';

-- name: ListSpecs :many
-- Lists specs ordered by created_at descending (most recent first).
-- There is an index on created_at to optimize this query.
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM specs
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ListLatestNamedSpecs :many
WITH latest AS (
  SELECT DISTINCT ON (name, source->>'domain', source->>'repo')
    id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
  FROM specs
  WHERE name <> '' AND sha <> ''
  ORDER BY name, source->>'domain', source->>'repo', source_committed_at DESC NULLS LAST, created_at DESC, id DESC
)
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM latest
ORDER BY source_committed_at DESC NULLS LAST, created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ResolveLatestNamedSpecByName :many
WITH latest AS (
  SELECT DISTINCT ON (name, source->>'domain', source->>'repo')
    id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
  FROM specs
  WHERE name = sqlc.arg(name)::text AND sha <> ''
  ORDER BY name, source->>'domain', source->>'repo', source_committed_at DESC NULLS LAST, created_at DESC, id DESC
)
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM latest
ORDER BY source->>'domain', source->>'repo', name;

-- name: ResolveLatestNamedSpecByRepoName :many
WITH latest AS (
  SELECT DISTINCT ON (name, source->>'domain', source->>'repo')
    id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
  FROM specs
  WHERE name = sqlc.arg(name)::text
    AND source->>'repo' = sqlc.arg(repo)::text
    AND sha <> ''
  ORDER BY name, source->>'domain', source->>'repo', source_committed_at DESC NULLS LAST, created_at DESC, id DESC
)
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM latest
ORDER BY source->>'domain', source->>'repo', name;

-- name: ResolveLatestNamedSpecByDomainRepoName :many
WITH latest AS (
  SELECT DISTINCT ON (name, source->>'domain', source->>'repo')
    id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
  FROM specs
  WHERE name = sqlc.arg(name)::text
    AND source->>'domain' = sqlc.arg(domain)::text
    AND source->>'repo' = sqlc.arg(repo)::text
    AND sha <> ''
  ORDER BY name, source->>'domain', source->>'repo', source_committed_at DESC NULLS LAST, created_at DESC, id DESC
)
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at, archived_at
FROM latest
ORDER BY source->>'domain', source->>'repo', name;
