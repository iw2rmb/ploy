-- name: ListSBOMCompatRows :many
WITH sbom_rows_with_stack AS (
  SELECT
    s.lib,
    s.ver,
    st.lang,
    st.release,
    COALESCE(st.tool, '') AS tool
  FROM sboms s
  JOIN jobs j ON j.id = s.job_id
  JOIN gates g ON g.job_id = CASE
    WHEN j.job_type = 'sbom' THEN (
      SELECT gj.id
      FROM jobs gj
      WHERE gj.run_id = j.run_id
        AND gj.repo_id = j.repo_id
        AND gj.attempt = j.attempt
        AND gj.name = regexp_replace(j.name, '-sbom$', '')
        AND gj.job_type IN ('pre_gate', 'post_gate', 're_gate')
      LIMIT 1
    )
    ELSE j.id
  END
  JOIN gate_profiles gp ON gp.id = g.profile_id
  JOIN stacks st ON st.id = gp.stack_id
  WHERE j.status = 'Success'
    AND j.job_type IN ('pre_gate', 'post_gate', 're_gate', 'sbom')
)
SELECT s.lib, s.ver
FROM sbom_rows_with_stack s
WHERE s.lang = sqlc.arg(lang)::text
  AND s.release = sqlc.arg(release)::text
  AND s.tool = sqlc.arg(tool)::text
  AND s.lib = ANY(sqlc.arg(libs)::text[])
GROUP BY s.lib, s.ver
ORDER BY s.lib ASC, s.ver ASC;

-- name: HasSBOMEvidenceForStack :one
WITH sbom_rows_with_stack AS (
  SELECT
    st.lang,
    st.release,
    COALESCE(st.tool, '') AS tool
  FROM sboms s
  JOIN jobs j ON j.id = s.job_id
  JOIN gates g ON g.job_id = CASE
    WHEN j.job_type = 'sbom' THEN (
      SELECT gj.id
      FROM jobs gj
      WHERE gj.run_id = j.run_id
        AND gj.repo_id = j.repo_id
        AND gj.attempt = j.attempt
        AND gj.name = regexp_replace(j.name, '-sbom$', '')
        AND gj.job_type IN ('pre_gate', 'post_gate', 're_gate')
      LIMIT 1
    )
    ELSE j.id
  END
  JOIN gate_profiles gp ON gp.id = g.profile_id
  JOIN stacks st ON st.id = gp.stack_id
  WHERE j.status = 'Success'
    AND j.job_type IN ('pre_gate', 'post_gate', 're_gate', 'sbom')
)
SELECT EXISTS(
  SELECT 1
  FROM sbom_rows_with_stack s
  WHERE s.lang = sqlc.arg(lang)::text
    AND s.release = sqlc.arg(release)::text
    AND s.tool = sqlc.arg(tool)::text
) AS has_evidence;
