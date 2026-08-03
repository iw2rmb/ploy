-- name: CreateSpec :one
INSERT INTO specs (id, name, spec, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, source, sha, source_committed_at, spec, created_by, created_at;

-- name: CreateGitSpecSnapshot :one
INSERT INTO specs (id, name, description, source, sha, source_committed_at, spec, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, description, source, sha, source_committed_at, spec, created_by, created_at;

-- name: GetSpec :one
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at
FROM specs
WHERE id = $1;

-- name: GetGitSpecSnapshot :one
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at
FROM specs
WHERE name = sqlc.arg(name)::text
  AND source->>'domain' = sqlc.arg(domain)::text
  AND source->>'repo' = sqlc.arg(repo)::text
  AND COALESCE(source->>'path', '') = sqlc.arg(path)::text
  AND sha = sqlc.arg(sha)::text
  AND sha <> ''
  AND spec = sqlc.arg(spec)::jsonb
ORDER BY created_at, id
LIMIT 1;

-- name: ListSpecs :many
-- Lists specs ordered by created_at descending (most recent first).
-- There is an index on created_at to optimize this query.
SELECT id, name, description, source, sha, source_committed_at, spec, created_by, created_at
FROM specs
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;
