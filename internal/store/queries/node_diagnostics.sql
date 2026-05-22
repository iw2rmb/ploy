-- name: UpsertNodeDiagnostic :one
INSERT INTO node_diagnostics (
  node_id,
  component,
  status,
  last_error,
  version,
  image_ref,
  local_image_id,
  remote_image_id,
  details,
  last_checked_at,
  last_success_at,
  updated_at
) VALUES (
  sqlc.arg(node_id),
  sqlc.arg(component),
  sqlc.arg(status),
  sqlc.narg(last_error),
  sqlc.narg(version),
  sqlc.narg(image_ref),
  sqlc.narg(local_image_id),
  sqlc.narg(remote_image_id),
  sqlc.arg(details),
  sqlc.narg(last_checked_at),
  sqlc.narg(last_success_at),
  now()
)
ON CONFLICT (node_id, component) DO UPDATE
SET
  status = EXCLUDED.status,
  last_error = EXCLUDED.last_error,
  version = EXCLUDED.version,
  image_ref = EXCLUDED.image_ref,
  local_image_id = EXCLUDED.local_image_id,
  remote_image_id = EXCLUDED.remote_image_id,
  details = EXCLUDED.details,
  last_checked_at = EXCLUDED.last_checked_at,
  last_success_at = EXCLUDED.last_success_at,
  updated_at = now()
RETURNING *;

-- name: ListNodeDiagnostics :many
SELECT * FROM node_diagnostics
WHERE node_id = sqlc.arg(node_id)
ORDER BY component;

-- name: CreateNodeDaemonLog :one
INSERT INTO node_daemon_logs (
  node_id,
  component,
  stream,
  message
) VALUES (
  sqlc.arg(node_id),
  sqlc.arg(component),
  sqlc.arg(stream),
  sqlc.arg(message)
)
RETURNING *;

-- name: ListNodeDaemonLogs :many
SELECT * FROM node_daemon_logs
WHERE node_id = sqlc.arg(node_id)
  AND (sqlc.narg(component)::text IS NULL OR component = sqlc.narg(component)::text)
ORDER BY id DESC
LIMIT sqlc.arg(limit_count);

-- name: TrimNodeDaemonLogs :exec
DELETE FROM node_daemon_logs
WHERE node_daemon_logs.node_id = sqlc.arg(node_id)
  AND node_daemon_logs.component = sqlc.arg(component)
  AND id NOT IN (
  SELECT id
  FROM node_daemon_logs
  WHERE node_daemon_logs.node_id = sqlc.arg(node_id)
    AND node_daemon_logs.component = sqlc.arg(component)
  ORDER BY id DESC
  LIMIT sqlc.arg(keep_count)
);
