-- name: UpsertSBOMStep :exec
INSERT INTO sbom_steps (job_id, lang, tool, release, ref_job_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (job_id)
DO UPDATE SET
  lang = EXCLUDED.lang,
  tool = EXCLUDED.tool,
  release = EXCLUDED.release,
  ref_job_id = EXCLUDED.ref_job_id;

-- name: ResolveReusableSBOMByRepoSHAAndStack :one
SELECT
  j.id AS ref_job_id,
  j.job_image AS ref_job_image,
  ab.id::text AS ref_artifact_id
FROM sbom_steps ss
JOIN jobs j ON j.id = ss.job_id
JOIN LATERAL (
  SELECT a.id
  FROM artifact_bundles a
  WHERE a.run_id = j.run_id
    AND a.job_id = j.id
    AND (a.name = 'mig-out' OR a.name IS NULL)
  ORDER BY a.created_at DESC, a.id DESC
  LIMIT 1
) ab ON true
WHERE j.repo_id = $1
  AND j.repo_sha_in = $2
  AND j.job_type = 'sbom'
  AND j.status = 'Success'
  AND ss.lang = $3
  AND ss.tool = $4
  AND ss.release = $5
  AND j.job_image <> ''
ORDER BY j.finished_at DESC NULLS LAST, j.id DESC
LIMIT 1;
