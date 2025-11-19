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

-- name: InsertAPIToken :exec
INSERT INTO api_tokens (
    token_hash,
    token_id,
    cluster_id,
    role,
    description,
    issued_at,
    expires_at,
    created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: ListAPITokens :many
SELECT
    token_id,
    role,
    description,
    issued_at,
    expires_at,
    last_used_at,
    revoked_at,
    created_by
FROM api_tokens
WHERE cluster_id = $1
ORDER BY created_at DESC;

-- name: RevokeAPIToken :exec
UPDATE api_tokens
SET revoked_at = NOW()
WHERE token_id = $1;

-- name: InsertBootstrapToken :exec
INSERT INTO bootstrap_tokens (
    token_hash,
    token_id,
    node_id,
    cluster_id,
    issued_at,
    expires_at,
    issued_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: GetBootstrapToken :one
SELECT
    node_id,
    cluster_id,
    issued_at,
    expires_at,
    used_at,
    cert_issued_at,
    revoked_at
FROM bootstrap_tokens
WHERE token_id = $1
LIMIT 1;

-- name: MarkBootstrapTokenCertIssued :exec
UPDATE bootstrap_tokens
SET cert_issued_at = NOW()
WHERE token_id = $1;
