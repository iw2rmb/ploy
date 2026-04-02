-- config_env.sql — CRUD queries for global environment variables (config_env table).
-- Provides ListGlobalEnv, GetGlobalEnv, UpsertGlobalEnv, DeleteGlobalEnv.

-- name: ListGlobalEnv :many
-- Returns all global environment entries, ordered by key then target for consistent iteration.
-- Used by ConfigHolder initialization and HTTP list endpoint.
SELECT key, target, value, secret, updated_at
FROM config_env
ORDER BY key ASC, target ASC;

-- name: GetGlobalEnv :one
-- Retrieves a single environment entry by key and target.
-- Returns pgx.ErrNoRows if the (key, target) pair does not exist.
SELECT key, target, value, secret, updated_at
FROM config_env
WHERE key = $1 AND target = $2;

-- name: UpsertGlobalEnv :exec
-- Inserts or updates an environment entry (upsert on composite key (key, target)).
-- Updates value, secret, and refreshes updated_at on conflict.
-- This ensures idempotent set operations from the CLI or API.
INSERT INTO config_env (key, target, value, secret, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (key, target) DO UPDATE SET
  value      = EXCLUDED.value,
  secret     = EXCLUDED.secret,
  updated_at = now();

-- name: DeleteGlobalEnv :exec
-- Removes an environment entry by key and target.
-- No-op if the (key, target) pair does not exist (exec returns no error).
DELETE FROM config_env
WHERE key = $1 AND target = $2;
