-- name: CreateSpec :one
INSERT INTO specs (id, name, spec, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, spec, created_by, created_at, archived_at;

-- name: GetSpec :one
SELECT id, name, spec, created_by, created_at, archived_at
FROM specs
WHERE id = $1;

-- name: ListSpecs :many
-- Lists specs ordered by created_at descending (most recent first).
-- Per roadmap/v1/db.md:53, there is an index on created_at for this query.
SELECT id, name, spec, created_by, created_at, archived_at
FROM specs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

