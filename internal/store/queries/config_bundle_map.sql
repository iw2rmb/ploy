-- config_bundle_map.sql — read queries for global bundle map entries (config_bundle_map table).
-- Provides ListConfigBundleMap.

-- name: ListConfigBundleMap :many
-- Returns all bundle map entries ordered by hash for deterministic iteration.
SELECT hash, bundle_id, updated_at
FROM config_bundle_map
ORDER BY hash ASC;
