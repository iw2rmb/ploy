-- name: GetNode :one
SELECT * FROM nodes
WHERE id = $1;

-- name: ListNodes :many
SELECT * FROM nodes
ORDER BY created_at DESC, id DESC;

-- name: CreateNode :one
-- Creates a new node with an application-supplied NanoID(6) as the primary key.
-- The `id` parameter must be generated via types.NewNodeKey() before calling.
INSERT INTO nodes (
  id,
  name,
  ip_address,
  version,
  concurrency
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: UpdateNodeHeartbeat :exec
WITH updated AS (
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
  WHERE nodes.id = $1
  RETURNING nodes.id
)
INSERT INTO node_metrics (
  node_id,
  cpu_total_millis,
  cpu_free_millis,
  mem_total_bytes,
  mem_free_bytes,
  disk_total_bytes,
  disk_free_bytes
)
SELECT
  updated.id,
  $3,
  $4,
  $5,
  $6,
  $7,
  $8
FROM updated;

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
