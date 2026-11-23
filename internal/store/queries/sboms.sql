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

-- name: ListSBOMJVMVersions :many
WITH eligible_jobs AS (
  SELECT id FROM jobs
  WHERE job_type IN ('pre_gate', 'post_gate')
    AND status = 'Success'
    AND finished_at >= ((statement_timestamp() AT TIME ZONE 'UTC' - interval '9 months') AT TIME ZONE 'UTC')
    AND meta #>> '{gate,detected_stack,language}' = 'java'
    AND meta #>> '{gate,detected_stack,release}' = sqlc.arg(jvm)::text
), requested AS (
  SELECT p.value->>'lib' AS lib, p.value->>'kind' AS kind,
    p.value->>'value' AS value, p.value->>'membership' AS membership
  FROM jsonb_array_elements(sqlc.arg(requested)::jsonb) AS p
)
SELECT
  (array_agg(s.lib || '@' || s.ver) FILTER (WHERE q.membership = 'for'))::text[] AS found,
  coalesce(array_agg(s.lib || '@' || s.ver) FILTER (WHERE q.membership = 'with'), '{}')::text[] AS "with"
FROM eligible_jobs j
JOIN sboms s ON s.job_id = j.id
JOIN requested q ON q.lib = s.lib
  AND (q.kind = 'any' OR (q.kind = 'exact' AND s.ver = q.value)
    OR (q.kind = 'prefix' AND starts_with(s.ver, q.value)))
GROUP BY s.job_id
HAVING count(DISTINCT s.lib) = (SELECT count(*) FROM requested);
