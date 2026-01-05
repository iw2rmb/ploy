-- name: CreateMod :one
INSERT INTO mods (id, name, spec_id, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, spec_id, created_by, created_at, archived_at;

-- name: GetMod :one
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM mods
WHERE id = $1;

-- name: UpdateModSpec :exec
UPDATE mods
SET spec_id = $2
WHERE id = $1;

