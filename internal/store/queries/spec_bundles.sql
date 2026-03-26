-- name: CreateSpecBundle :one
-- Creates a new spec bundle metadata row. Blob data is stored in object storage.
-- The returned id becomes the bundle_id field in TmpBundleRef.
INSERT INTO spec_bundles (id, cid, digest, size, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, cid, digest, size, object_key, created_by, created_at, last_ref_at;

-- name: GetSpecBundle :one
-- Returns spec bundle metadata including object_key for object-storage retrieval.
SELECT id, cid, digest, size, object_key, created_by, created_at, last_ref_at
FROM spec_bundles
WHERE id = $1;

-- name: GetSpecBundleByCID :one
-- Returns the most recently created spec bundle for a given cid.
-- Used for deduplication: callers should check by CID before uploading.
SELECT id, cid, digest, size, object_key, created_by, created_at, last_ref_at
FROM spec_bundles
WHERE cid = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSpecBundles :many
-- Lists spec bundles ordered by created_at descending (most recent first).
SELECT id, cid, digest, size, object_key, created_by, created_at, last_ref_at
FROM spec_bundles
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateSpecBundleLastRefAt :exec
-- Updates last_ref_at to now() for the given spec bundle.
-- Call this whenever a spec or run references the bundle to keep GC metadata fresh.
UPDATE spec_bundles
SET last_ref_at = now()
WHERE id = $1;

-- name: DeleteSpecBundle :exec
-- Deletes a spec bundle metadata row by ID.
-- Called by blobpersist as rollback when object storage upload fails.
DELETE FROM spec_bundles WHERE id = $1;

-- name: ListSpecBundlesUnreferencedBefore :many
-- Lists spec bundles whose last_ref_at is before the given threshold.
-- Used by GC to find bundles eligible for deletion.
SELECT id, cid, digest, size, object_key, created_by, created_at, last_ref_at
FROM spec_bundles
WHERE last_ref_at < $1
ORDER BY last_ref_at ASC;
