-- config_bundle_map.sql — CRUD queries for global bundle map entries (config_bundle_map table).
-- Provides ListConfigBundleMap, UpsertConfigBundleMap, DeleteConfigBundleMap.

-- name: ListConfigBundleMap :many
-- Returns all bundle map entries ordered by hash for deterministic iteration.
SELECT hash, bundle_id, updated_at
FROM config_bundle_map
ORDER BY hash ASC;

-- name: UpsertConfigBundleMap :exec
-- Inserts or updates a bundle map entry (upsert on primary key hash).
-- Refreshes bundle_id and updated_at on conflict.
INSERT INTO config_bundle_map (hash, bundle_id, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (hash) DO UPDATE SET
  bundle_id  = EXCLUDED.bundle_id,
  updated_at = now();

-- name: DeleteConfigBundleMap :exec
-- Removes a bundle map entry by hash.
DELETE FROM config_bundle_map
WHERE hash = $1;
