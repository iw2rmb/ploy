-- name: CreateMig :one
INSERT INTO migs (id, name, spec_id, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, spec_id, created_by, created_at, archived_at;

-- name: GetMig :one
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM migs
WHERE id = $1;

-- name: GetMigByName :one
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM migs
WHERE name = $1;

-- name: ListMigs :many
-- Lists migs with optional filtering by archived status and name substring.
-- archived_only: if true, return only archived migs; if false, return only active migs; if null, return all.
-- name_filter: if non-empty, filter by name substring (case-insensitive); if null/empty, no name filtering.
SELECT id, name, spec_id, created_by, created_at, archived_at
FROM migs
WHERE (sqlc.narg(archived_only)::boolean IS NULL OR
       (sqlc.narg(archived_only)::boolean = true AND archived_at IS NOT NULL) OR
       (sqlc.narg(archived_only)::boolean = false AND archived_at IS NULL))
  AND (sqlc.narg(name_filter)::text IS NULL OR sqlc.narg(name_filter)::text = '' OR name ILIKE '%' || sqlc.narg(name_filter)::text || '%')
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateMigSpec :exec
UPDATE migs
SET spec_id = $2
WHERE id = $1;

-- name: ArchiveMig :exec
-- Archives a mig by setting archived_at to now().
-- Archiving must be refused when the mig has any jobs in a running state.
-- This query only sets the timestamp; validation logic must be in the caller.
UPDATE migs
SET archived_at = now()
WHERE id = $1 AND archived_at IS NULL;

-- name: UnarchiveMig :exec
-- Unarchives a mig by clearing archived_at.
UPDATE migs
SET archived_at = NULL
WHERE id = $1 AND archived_at IS NOT NULL;

-- name: DeleteMig :exec
-- Deletes a mig. Use with caution; should only be called when safe to remove.
DELETE FROM migs
WHERE id = $1;
