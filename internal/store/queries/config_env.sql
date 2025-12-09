-- config_env.sql — CRUD queries for global environment variables (config_env table).
-- Per ROADMAP.md line 10-44: provides ListGlobalEnv, GetGlobalEnv, UpsertGlobalEnv, DeleteGlobalEnv.

-- name: ListGlobalEnv :many
-- Returns all global environment entries, ordered by key for consistent iteration.
-- Used by ConfigHolder initialization and HTTP list endpoint.
SELECT key, value, scope, secret, updated_at
FROM config_env
ORDER BY key ASC;

-- name: GetGlobalEnv :one
-- Retrieves a single environment entry by key.
-- Returns pgx.ErrNoRows if the key does not exist.
SELECT key, value, scope, secret, updated_at
FROM config_env
WHERE key = $1;

-- name: UpsertGlobalEnv :exec
-- Inserts or updates an environment entry (upsert on primary key 'key').
-- Updates value, scope, secret, and refreshes updated_at on conflict.
-- This ensures idempotent set operations from the CLI or API.
INSERT INTO config_env (key, value, scope, secret, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (key) DO UPDATE SET
  value      = EXCLUDED.value,
  scope      = EXCLUDED.scope,
  secret     = EXCLUDED.secret,
  updated_at = now();

-- name: DeleteGlobalEnv :exec
-- Removes an environment entry by key.
-- No-op if the key does not exist (exec returns no error).
DELETE FROM config_env
WHERE key = $1;
