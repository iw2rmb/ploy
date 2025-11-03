-- name: GetNode :one
SELECT * FROM nodes
WHERE id = $1;

-- name: ListNodes :many
SELECT * FROM nodes
ORDER BY created_at DESC;

-- name: CreateNode :one
INSERT INTO nodes (
  name,
  ip_address,
  version,
  concurrency
) VALUES (
  $1, $2, $3, $4
)
RETURNING *;

-- name: UpdateNodeHeartbeat :exec
UPDATE nodes
SET
  last_heartbeat = $2,
  cpu_total_millis = $3,
  cpu_free_millis = $4,
  mem_total_bytes = $5,
  mem_free_bytes = $6,
  disk_total_bytes = $7,
  disk_free_bytes = $8,
  -- Update version only when provided (non-empty); keep existing otherwise.
  version = COALESCE(NULLIF(sqlc.arg(version)::text, ''), version)
WHERE id = $1;

-- name: DeleteNode :exec
DELETE FROM nodes
WHERE id = $1;

-- name: UpdateNodeCertMetadata :exec
UPDATE nodes
SET
  cert_serial = $2,
  cert_fingerprint = $3,
  cert_not_before = $4,
  cert_not_after = $5
WHERE id = $1;

-- name: UpdateNodeDrained :exec
UPDATE nodes
SET drained = $2
WHERE id = $1;
