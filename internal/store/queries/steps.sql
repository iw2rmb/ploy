-- name: UpsertStep :exec
INSERT INTO steps (job_id, ops, hash, ref_job_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (job_id)
DO UPDATE SET
  ops = EXCLUDED.ops,
  hash = EXCLUDED.hash,
  ref_job_id = EXCLUDED.ref_job_id;

-- name: GetStepByJob :one
SELECT job_id, ops, hash, ref_job_id, created_at
FROM steps
WHERE job_id = $1;

-- name: ResolveReusableStepByHash :one
SELECT
  j.id AS ref_job_id,
  j.repo_sha_out AS ref_repo_sha_out
FROM steps s
JOIN jobs j ON j.id = s.job_id
WHERE j.repo_id = $1
  AND j.repo_sha_in = $2
  AND j.job_type = 'mig'
  AND j.status = 'Success'
  AND j.repo_sha_out ~ '^[0-9a-f]{40}$'
  AND s.hash = $3
ORDER BY j.finished_at DESC NULLS LAST, j.id DESC
LIMIT 1;
