-- name: UpsertSBOMRow :exec
INSERT INTO sboms (job_id, repo_id, lib, ver)
VALUES ($1, $2, $3, $4)
ON CONFLICT (job_id, repo_id, lib, ver) DO NOTHING;

-- name: DeleteSBOMRowsByJob :exec
DELETE FROM sboms
WHERE job_id = $1;

-- name: ListRunSBOMRowsByJobType :many
SELECT DISTINCT s.lib, s.ver
FROM sboms s
JOIN jobs j ON j.id = s.job_id
JOIN runs r ON r.id = j.run_id
WHERE j.run_id = $1
  AND j.attempt = r.attempt
  AND j.job_type = $2
ORDER BY s.lib ASC, s.ver ASC;
