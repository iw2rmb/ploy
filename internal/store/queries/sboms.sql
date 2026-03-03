-- name: UpsertSBOMRow :exec
INSERT INTO sboms (job_id, repo_id, lib, ver)
VALUES ($1, $2, $3, $4)
ON CONFLICT (job_id, repo_id, lib, ver) DO NOTHING;

-- name: DeleteSBOMRowsByJob :exec
DELETE FROM sboms
WHERE job_id = $1;

-- name: ListSBOMRowsByJob :many
SELECT job_id, repo_id, lib, ver, created_at
FROM sboms
WHERE job_id = $1
ORDER BY lib ASC, ver ASC;
