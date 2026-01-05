-- name: CreateSpec :one
INSERT INTO specs (id, name, spec, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, spec, created_by, created_at, archived_at;

-- name: GetSpec :one
SELECT id, name, spec, created_by, created_at, archived_at
FROM specs
WHERE id = $1;

