-- name: CheckAPITokenRevoked :one
SELECT revoked_at FROM api_tokens
WHERE token_id = $1 AND revoked_at IS NOT NULL
LIMIT 1;

-- name: CheckBootstrapTokenRevoked :one
SELECT revoked_at FROM bootstrap_tokens
WHERE token_id = $1 AND revoked_at IS NOT NULL
LIMIT 1;

-- name: UpdateAPITokenLastUsed :exec
UPDATE api_tokens
SET last_used_at = NOW()
WHERE token_id = $1;

-- name: UpdateBootstrapTokenLastUsed :exec
UPDATE bootstrap_tokens
SET used_at = NOW()
WHERE token_id = $1 AND used_at IS NULL;
