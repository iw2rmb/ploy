-- name: GetArtifactBundle :one
SELECT * FROM artifact_bundles
WHERE id = $1;

-- name: ListArtifactBundlesByRun :many
SELECT * FROM artifact_bundles
WHERE run_id = $1
ORDER BY created_at DESC;

-- name: ListArtifactBundlesByRunAndJob :many
SELECT * FROM artifact_bundles
WHERE run_id = $1 AND job_id = $2
ORDER BY created_at DESC;

-- name: CreateArtifactBundle :one
INSERT INTO artifact_bundles (run_id, job_id, build_id, name, bundle, cid, digest)
VALUES ($1, $2, $3, $4, $5, $6, $7)
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
ORDER BY created_at DESC;
