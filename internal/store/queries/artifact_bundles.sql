-- name: GetArtifactBundle :one
-- Returns artifact bundle metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE id = $1;

-- name: ListArtifactBundlesByRun :many
-- Returns artifact bundle metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = sqlc.arg(run_id)
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesByRunAndJob :many
-- Returns artifact bundle metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = sqlc.arg(run_id) AND job_id = sqlc.arg(job_id)
ORDER BY created_at DESC, id DESC;

-- name: CreateArtifactBundle :one
-- Creates a new artifact bundle metadata. Blob data is stored in object storage.
-- Bundles are grouped at the job level only (build_id removed).
INSERT INTO artifact_bundles (run_id, job_id, name, bundle_size, cid, digest)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteArtifactBundle :exec
DELETE FROM artifact_bundles
WHERE id = $1;

-- name: DeleteArtifactBundlesOlderThan :exec
DELETE FROM artifact_bundles
WHERE created_at < $1;

-- name: ListArtifactBundlesByCID :many
-- Returns artifact bundle metadata including object_key for object-storage retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE cid = sqlc.arg(cid)
ORDER BY created_at DESC, id DESC;
