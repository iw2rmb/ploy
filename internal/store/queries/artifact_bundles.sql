-- name: GetArtifactBundle :one
-- Returns artifact bundle metadata including object_key for MinIO retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE id = $1;

-- name: ListArtifactBundlesByRun :many
-- Returns artifact bundle metadata including object_key for MinIO retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesByRunAndJob :many
-- Returns artifact bundle metadata including object_key for MinIO retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = $1 AND job_id = $2
ORDER BY created_at DESC, id DESC;

-- name: CreateArtifactBundle :one
-- Creates a new artifact bundle metadata. Blob data is stored in MinIO.
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
-- Returns artifact bundle metadata including object_key for MinIO retrieval.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE cid = $1
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesMetaByCID :many
-- Returns artifact bundle metadata for a given cid.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE cid = $1
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesMetaByRun :many
-- Returns artifact bundle metadata for a run.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListArtifactBundlesMetaByRunAndJob :many
-- Returns artifact bundle metadata for a run and job.
SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at FROM artifact_bundles
WHERE run_id = $1 AND job_id = $2
ORDER BY created_at DESC, id DESC;
