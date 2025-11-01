-- name: GetMod :one
SELECT * FROM mods
WHERE id = $1;

-- name: ListMods :many
SELECT * FROM mods
ORDER BY created_at DESC;

-- name: ListModsByRepo :many
SELECT * FROM mods
WHERE repo_id = $1
ORDER BY created_at DESC;

-- name: CreateMod :one
INSERT INTO mods (repo_id, spec, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteMod :exec
DELETE FROM mods
WHERE id = $1;
