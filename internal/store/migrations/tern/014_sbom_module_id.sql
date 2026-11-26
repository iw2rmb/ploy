ALTER TABLE ploy.sboms
  ADD COLUMN module_id TEXT CHECK (module_id <> ''),
  DROP CONSTRAINT sboms_pkey;

CREATE UNIQUE INDEX sboms_job_module_package_idx
  ON ploy.sboms(job_id, repo_id, COALESCE(module_id, ''), lib, ver);

---- create above / drop below ----

-- A package shared by several modules becomes one row in the legacy schema.
DELETE FROM ploy.sboms s
USING ploy.sboms other
WHERE (s.job_id, s.repo_id, s.lib, s.ver) = (other.job_id, other.repo_id, other.lib, other.ver)
  AND s.ctid < other.ctid;

ALTER TABLE ploy.sboms
  DROP COLUMN module_id,
  ADD PRIMARY KEY (job_id, repo_id, lib, ver);
