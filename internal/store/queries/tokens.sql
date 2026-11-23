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

-- name: InsertAPIToken :exec
INSERT INTO api_tokens (
    token_hash,
    token_id,
    role,
    username,
    description,
    issued_at,
    expires_at,
    created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: GetAPITokenByID :one
SELECT
    token_id,
    role,
    username,
    description,
    issued_at,
    expires_at,
    last_used_at,
    revoked_at,
    created_by
FROM api_tokens
WHERE token_id = $1
LIMIT 1;

-- name: ListAPITokens :many
SELECT
    token_id,
    role,
    username,
    description,
    issued_at,
    expires_at,
    last_used_at,
    revoked_at,
    created_by
FROM api_tokens
WHERE job_id IS NULL
ORDER BY created_at DESC, token_id DESC;

-- name: RevokeAPIToken :exec
UPDATE api_tokens
SET revoked_at = NOW()
WHERE token_id = $1;

-- name: InsertBootstrapToken :exec
INSERT INTO bootstrap_tokens (
    token_hash,
    token_id,
    node_id,
    issued_at,
    expires_at,
    issued_by
) VALUES (
    $1, $2, $3, $4, $5, $6
);

-- name: GetBootstrapToken :one
SELECT
    node_id,
    issued_at,
    expires_at,
    used_at,
    cert_issued_at,
    revoked_at
FROM bootstrap_tokens
WHERE token_id = $1
LIMIT 1;

-- name: UpsertJobToken :one
-- Bind issued_at to this claim so a later claim invalidates the previous token
-- immediately, including the interval before the new token is issued.
INSERT INTO api_tokens (job_id, role, token_id, token_hash, expires_at, issued_at)
SELECT j.id, 'job', @token_id, @token_hash, @expires_at, j.started_at
FROM jobs j
WHERE j.id = @job_id AND j.status = 'Running' AND j.started_at = @started_at
FOR UPDATE OF j
ON CONFLICT (job_id) DO UPDATE
SET token_id = EXCLUDED.token_id, token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at, issued_at = EXCLUDED.issued_at,
    revoked_at = NULL, last_used_at = NULL
RETURNING token_id;

-- name: IsJobTokenActive :one
SELECT EXISTS (
  SELECT 1 FROM api_tokens t JOIN jobs j ON t.job_id = j.id
  WHERE t.token_id = @token_id AND t.role = 'job' AND t.revoked_at IS NULL
    AND j.status = 'Running' AND t.issued_at = j.started_at AND t.expires_at > now()
);
