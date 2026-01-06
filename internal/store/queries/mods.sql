-- name: CreateMod :one
INSERT INTO mods (id, name, spec_id, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, spec_id, created_by, created_at, archived_at;

-- name: GetMod :one
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM mods
WHERE id = $1;

-- name: GetModByName :one
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM mods
WHERE name = $1;

-- name: ListMods :many
-- Lists mods with optional filtering by archived status and name substring.
-- archived_only: if true, return only archived mods; if false, return only active mods; if null, return all.
-- name_filter: if non-empty, filter by name substring (case-insensitive); if null/empty, no name filtering.
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM mods
WHERE (sqlc.narg(archived_only)::boolean IS NULL OR
       (sqlc.narg(archived_only)::boolean = true AND archived_at IS NOT NULL) OR
       (sqlc.narg(archived_only)::boolean = false AND archived_at IS NULL))
  AND (sqlc.narg(name_filter)::text IS NULL OR sqlc.narg(name_filter)::text = '' OR name ILIKE '%' || sqlc.narg(name_filter)::text || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateModSpec :exec
UPDATE mods
SET spec_id = $2
WHERE id = $1;

-- name: ArchiveMod :exec
-- Archives a mod by setting archived_at to now().
-- Archiving must be refused when the mod has any jobs in a running state.
-- This query only sets the timestamp; validation logic must be in the caller.
UPDATE mods
SET archived_at = now()
WHERE id = $1 AND archived_at IS NULL;

-- name: UnarchiveMod :exec
-- Unarchives a mod by clearing archived_at.
UPDATE mods
SET archived_at = NULL
WHERE id = $1 AND archived_at IS NOT NULL;

-- name: DeleteMod :exec
-- Deletes a mod. Use with caution; should only be called when safe to remove.
DELETE FROM mods
WHERE id = $1;
