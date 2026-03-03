-- name: ListSBOMCompatRows :many
SELECT s.lib, s.ver
FROM sboms s
JOIN jobs j ON j.id = s.job_id
JOIN gates g ON g.job_id = s.job_id
JOIN gate_profiles gp ON gp.id = g.profile_id
JOIN stacks st ON st.id = gp.stack_id
WHERE j.status = 'Success'
  AND j.job_type IN ('pre_gate', 'post_gate', 're_gate')
  AND st.lang = sqlc.arg(lang)::text
  AND st.release = sqlc.arg(release)::text
  AND COALESCE(st.tool, '') = sqlc.arg(tool)::text
  AND s.lib = ANY(sqlc.arg(libs)::text[])
GROUP BY s.lib, s.ver
ORDER BY s.lib ASC, s.ver ASC;

-- name: HasSBOMEvidenceForStack :one
SELECT EXISTS(
  SELECT 1
  FROM sboms s
  JOIN jobs j ON j.id = s.job_id
  JOIN gates g ON g.job_id = s.job_id
  JOIN gate_profiles gp ON gp.id = g.profile_id
  JOIN stacks st ON st.id = gp.stack_id
  WHERE j.status = 'Success'
    AND j.job_type IN ('pre_gate', 'post_gate', 're_gate')
    AND st.lang = sqlc.arg(lang)::text
    AND st.release = sqlc.arg(release)::text
    AND COALESCE(st.tool, '') = sqlc.arg(tool)::text
) AS has_evidence;
