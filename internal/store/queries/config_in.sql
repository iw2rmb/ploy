-- config_in.sql — CRUD queries for global in mount entries (config_in table).
-- Provides ListConfigIn, UpsertConfigIn, DeleteConfigIn, DeleteConfigInBySection.

-- name: ListConfigIn :many
-- Returns all in entries ordered by section then dst for deterministic iteration.
SELECT entry, dst, section, updated_at
FROM config_in
ORDER BY section ASC, dst ASC;

-- name: ListConfigInBySection :many
-- Returns in entries for a specific section ordered by dst.
SELECT entry, dst, section, updated_at
FROM config_in
WHERE section = $1
ORDER BY dst ASC;

-- name: UpsertConfigIn :exec
-- Inserts or updates an in entry (upsert on composite key (dst, section)).
-- Refreshes entry and updated_at on conflict (entry may change if hash changes).
INSERT INTO config_in (entry, dst, section, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (dst, section) DO UPDATE SET
  entry      = EXCLUDED.entry,
  updated_at = now();

-- name: DeleteConfigIn :exec
-- Removes an in entry by dst and section.
DELETE FROM config_in
WHERE dst = $1 AND section = $2;

-- name: DeleteConfigInBySection :exec
-- Removes all in entries for a section.
DELETE FROM config_in
WHERE section = $1;
