-- name: GetArtifactBundle :one
SELECT * FROM artifact_bundles
WHERE id = $1;

-- name: ListArtifactBundlesByRun :many
SELECT * FROM artifact_bundles
WHERE run_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesByRunAndJob :many
SELECT * FROM artifact_bundles
WHERE run_id = $1 AND job_id = $2
ORDER BY created_at DESC, id DESC;

-- name: CreateArtifactBundle :one
-- Creates a new artifact bundle. Bundles are grouped at the job level only (build_id removed).
INSERT INTO artifact_bundles (run_id, job_id, name, bundle, cid, digest)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteArtifactBundle :exec
DELETE FROM artifact_bundles
WHERE id = $1;

-- name: DeleteArtifactBundlesOlderThan :exec
DELETE FROM artifact_bundles
WHERE created_at < $1;

-- name: ListArtifactBundlesByCID :many
SELECT * FROM artifact_bundles
WHERE cid = $1
ORDER BY created_at DESC, id DESC;
