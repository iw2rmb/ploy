-- config_in.sql — read queries for global in mount entries (config_in table).
-- Provides ListConfigIn.

-- name: ListConfigIn :many
-- Returns all in entries ordered by section then dst for deterministic iteration.
SELECT entry, dst, section, updated_at
FROM config_in
ORDER BY section ASC, dst ASC;
