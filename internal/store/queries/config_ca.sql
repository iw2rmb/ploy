-- config_ca.sql — CRUD queries for global CA hash entries (config_ca table).
-- Provides ListConfigCA, UpsertConfigCA, DeleteConfigCA, DeleteConfigCABySection.

-- name: ListConfigCA :many
-- Returns all CA entries ordered by section then hash for deterministic iteration.
SELECT hash, section, updated_at
FROM config_ca
ORDER BY section ASC, hash ASC;

-- name: ListConfigCABySection :many
-- Returns CA entries for a specific section ordered by hash.
SELECT hash, section, updated_at
FROM config_ca
WHERE section = $1
ORDER BY hash ASC;

-- name: UpsertConfigCA :exec
-- Inserts or updates a CA entry (upsert on composite key (hash, section)).
-- Refreshes updated_at on conflict.
INSERT INTO config_ca (hash, section, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (hash, section) DO UPDATE SET
  updated_at = now();

-- name: DeleteConfigCA :exec
-- Removes a CA entry by hash and section.
DELETE FROM config_ca
WHERE hash = $1 AND section = $2;

-- name: DeleteConfigCABySection :exec
-- Removes all CA entries for a section.
DELETE FROM config_ca
WHERE section = $1;
