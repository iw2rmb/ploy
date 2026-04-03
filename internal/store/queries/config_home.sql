-- config_home.sql — CRUD queries for global home mount entries (config_home table).
-- Provides ListConfigHome, UpsertConfigHome, DeleteConfigHome, DeleteConfigHomeBySection.

-- name: ListConfigHome :many
-- Returns all home entries ordered by section then dst for deterministic iteration.
SELECT entry, dst, section, updated_at
FROM config_home
ORDER BY section ASC, dst ASC;

-- name: ListConfigHomeBySection :many
-- Returns home entries for a specific section ordered by dst.
SELECT entry, dst, section, updated_at
FROM config_home
WHERE section = $1
ORDER BY dst ASC;

-- name: UpsertConfigHome :exec
-- Inserts or updates a home entry (upsert on composite key (dst, section)).
-- Refreshes entry and updated_at on conflict (entry may change if hash or ro flag changes).
INSERT INTO config_home (entry, dst, section, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (dst, section) DO UPDATE SET
  entry      = EXCLUDED.entry,
  updated_at = now();

-- name: DeleteConfigHome :exec
-- Removes a home entry by dst and section.
DELETE FROM config_home
WHERE dst = $1 AND section = $2;

-- name: DeleteConfigHomeBySection :exec
-- Removes all home entries for a section.
DELETE FROM config_home
WHERE section = $1;
