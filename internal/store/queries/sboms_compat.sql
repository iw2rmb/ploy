-- name: ListSBOMCompatRows :many
WITH latest_successful_gate_jobs AS (
  SELECT
    j.id,
    j.run_id,
    j.repo_id,
    j.attempt,
    ROW_NUMBER() OVER (
      PARTITION BY j.run_id, j.repo_id, j.attempt
      ORDER BY COALESCE(j.finished_at, j.started_at, j.created_at) DESC, j.id DESC
    ) AS rn
  FROM jobs j
  JOIN run_repos rr ON rr.run_id = j.run_id
    AND rr.repo_id = j.repo_id
    AND rr.attempt = j.attempt
  WHERE j.status = 'Success'
    AND j.job_type IN ('pre_gate', 'post_gate')
    AND rr.status = 'Success'
),
sbom_rows_with_stack AS (
  SELECT
    s.lib,
    s.ver,
    st.lang,
    st.release,
    COALESCE(st.tool, '') AS tool
  FROM latest_successful_gate_jobs j
  JOIN sboms s ON s.job_id = j.id
    AND s.repo_id = j.repo_id
  JOIN gates g ON g.job_id = j.id
  JOIN gate_profiles gp ON gp.id = g.profile_id
  JOIN stacks st ON st.id = gp.stack_id
  WHERE j.rn = 1
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
WITH latest_successful_gate_jobs AS (
  SELECT
    j.id,
    j.run_id,
    j.repo_id,
    j.attempt,
    ROW_NUMBER() OVER (
      PARTITION BY j.run_id, j.repo_id, j.attempt
      ORDER BY COALESCE(j.finished_at, j.started_at, j.created_at) DESC, j.id DESC
    ) AS rn
  FROM jobs j
  JOIN run_repos rr ON rr.run_id = j.run_id
    AND rr.repo_id = j.repo_id
    AND rr.attempt = j.attempt
  WHERE j.status = 'Success'
    AND j.job_type IN ('pre_gate', 'post_gate')
    AND rr.status = 'Success'
),
sbom_rows_with_stack AS (
  SELECT
    st.lang,
    st.release,
    COALESCE(st.tool, '') AS tool
  FROM latest_successful_gate_jobs j
  JOIN gates g ON g.job_id = j.id
  JOIN gate_profiles gp ON gp.id = g.profile_id
  JOIN stacks st ON st.id = gp.stack_id
  WHERE j.rn = 1
)
SELECT EXISTS(
  SELECT 1
  FROM sbom_rows_with_stack s
  WHERE s.lang = sqlc.arg(lang)::text
    AND s.release = sqlc.arg(release)::text
    AND s.tool = sqlc.arg(tool)::text
) AS has_evidence;
