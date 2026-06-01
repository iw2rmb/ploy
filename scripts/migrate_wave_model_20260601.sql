BEGIN;

LOCK TABLE ploy.runs, ploy.run_repos, ploy.jobs, ploy.migs IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
  active_jobs integer;
  multi_repo_runs integer;
  missing_job_runs integer;
BEGIN
  SELECT count(*) INTO active_jobs
  FROM ploy.jobs
  WHERE status IN ('Created','Queued','Running');
  IF active_jobs <> 0 THEN
    RAISE EXCEPTION 'wave migration preflight failed: % active jobs', active_jobs;
  END IF;

  SELECT count(*) INTO multi_repo_runs
  FROM (
    SELECT run_id
    FROM ploy.run_repos
    GROUP BY run_id
    HAVING count(*) <> 1
  ) x;
  IF multi_repo_runs <> 0 THEN
    RAISE EXCEPTION 'wave migration preflight failed: % non-single-repo runs', multi_repo_runs;
  END IF;

  SELECT count(*) INTO missing_job_runs
  FROM ploy.jobs j
  LEFT JOIN ploy.run_repos rr ON rr.run_id = j.run_id
    AND rr.repo_id = j.repo_id
    AND rr.attempt = j.attempt
  WHERE rr.run_id IS NULL;
  IF missing_job_runs <> 0 THEN
    RAISE EXCEPTION 'wave migration preflight failed: % jobs without run_repos', missing_job_runs;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ploy.runs_wave_backup_20260601 AS TABLE ploy.runs WITH DATA;
CREATE TABLE IF NOT EXISTS ploy.run_repos_wave_backup_20260601 AS TABLE ploy.run_repos WITH DATA;
CREATE TABLE IF NOT EXISTS ploy.jobs_wave_backup_20260601 AS TABLE ploy.jobs WITH DATA;
CREATE TABLE IF NOT EXISTS ploy.migs_wave_backup_20260601 AS TABLE ploy.migs WITH DATA;

DROP TABLE IF EXISTS ploy.wave_migration_orphan_runs_20260601;
CREATE TABLE ploy.wave_migration_orphan_runs_20260601 AS
SELECT r.*
FROM ploy.runs r
LEFT JOIN ploy.run_repos rr ON rr.run_id = r.id
WHERE rr.run_id IS NULL;

ALTER TYPE ploy.run_status RENAME TO wave_status;
ALTER TYPE ploy.run_repo_status RENAME TO run_status;

CREATE TABLE ploy.waves (
  id           TEXT PRIMARY KEY,
  mig_id       TEXT NOT NULL REFERENCES ploy.migs(id) ON DELETE RESTRICT,
  spec_id      TEXT NOT NULL REFERENCES ploy.specs(id) ON DELETE RESTRICT,
  created_by   TEXT,
  status       ploy.wave_status NOT NULL DEFAULT 'Started',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  stats        JSONB NOT NULL DEFAULT '{}'::jsonb
);

INSERT INTO ploy.waves (id, mig_id, spec_id, created_by, status, created_at, started_at, finished_at, stats)
SELECT r.id, r.mig_id, r.spec_id, r.created_by, r.status, r.created_at, r.started_at, r.finished_at, r.stats
FROM ploy.runs r
WHERE EXISTS (SELECT 1 FROM ploy.run_repos rr WHERE rr.run_id = r.id);

ALTER TABLE ploy.runs ADD COLUMN wave_id text;
ALTER TABLE ploy.runs ADD COLUMN repo_id text;
ALTER TABLE ploy.runs ADD COLUMN repo_base_ref text;
ALTER TABLE ploy.runs ADD COLUMN source_commit_sha text;
ALTER TABLE ploy.runs ADD COLUMN repo_sha0 text;
ALTER TABLE ploy.runs ADD COLUMN attempt integer;
ALTER TABLE ploy.runs ADD COLUMN last_error text;
ALTER TABLE ploy.runs ADD COLUMN execution_status ploy.run_status;

UPDATE ploy.runs r
SET wave_id = r.id,
    repo_id = rr.repo_id,
    repo_base_ref = rr.repo_base_ref,
    source_commit_sha = rr.source_commit_sha,
    repo_sha0 = rr.repo_sha0,
    attempt = rr.attempt,
    last_error = rr.last_error,
    created_at = rr.created_at,
    started_at = rr.started_at,
    finished_at = rr.finished_at,
    stats = '{}'::jsonb,
    execution_status = rr.status
FROM ploy.run_repos rr
WHERE rr.run_id = r.id;

DELETE FROM ploy.runs r
WHERE NOT EXISTS (SELECT 1 FROM ploy.run_repos rr WHERE rr.run_id = r.id);

ALTER TABLE ploy.runs DROP COLUMN status;
ALTER TABLE ploy.runs RENAME COLUMN execution_status TO status;

ALTER TABLE ploy.runs
  ALTER COLUMN wave_id SET NOT NULL,
  ALTER COLUMN repo_id SET NOT NULL,
  ALTER COLUMN repo_base_ref SET NOT NULL,
  ALTER COLUMN source_commit_sha SET NOT NULL,
  ALTER COLUMN repo_sha0 SET NOT NULL,
  ALTER COLUMN attempt SET NOT NULL,
  ALTER COLUMN status SET NOT NULL,
  ALTER COLUMN status SET DEFAULT 'Queued';

ALTER TABLE ploy.runs ADD CONSTRAINT runs_wave_fkey FOREIGN KEY (wave_id) REFERENCES ploy.waves(id) ON DELETE CASCADE;
ALTER TABLE ploy.runs ADD CONSTRAINT runs_repo_fkey FOREIGN KEY (repo_id) REFERENCES ploy.repos(id) ON DELETE RESTRICT;
ALTER TABLE ploy.runs ADD CONSTRAINT runs_mig_repo_membership_fkey FOREIGN KEY (mig_id, repo_id) REFERENCES ploy.mig_repos(mig_id, repo_id) ON DELETE RESTRICT;
ALTER TABLE ploy.runs ADD CONSTRAINT runs_attempt_check CHECK (attempt >= 1);

DROP TABLE ploy.run_repos;

ALTER TABLE IF EXISTS ploy.run_repo_actions RENAME TO run_actions;
ALTER TABLE IF EXISTS ploy.run_actions DROP COLUMN IF EXISTS repo_id;
DROP INDEX IF EXISTS ploy.run_repo_actions_pending_idx;
DROP INDEX IF EXISTS ploy.run_repo_actions_node_idx;
CREATE UNIQUE INDEX IF NOT EXISTS run_actions_run_attempt_type_uniq ON ploy.run_actions(run_id, attempt, action_type);
CREATE INDEX IF NOT EXISTS run_actions_pending_idx ON ploy.run_actions(run_id, attempt, id) WHERE status = 'Queued';
CREATE INDEX IF NOT EXISTS run_actions_node_idx ON ploy.run_actions(node_id) WHERE node_id IS NOT NULL;

CREATE INDEX waves_mig_created_idx ON ploy.waves(mig_id, created_at DESC, id DESC);
CREATE INDEX waves_status_idx ON ploy.waves(status) WHERE status IN ('Started');
CREATE INDEX runs_wave_created_idx ON ploy.runs(wave_id, created_at ASC, id ASC);
CREATE INDEX runs_status_idx ON ploy.runs(status) WHERE status IN ('Queued','Running');
CREATE INDEX runs_repo_created_idx ON ploy.runs(repo_id, created_at DESC, id DESC);
CREATE INDEX runs_mig_repo_created_idx ON ploy.runs(mig_id, repo_id, created_at DESC, id DESC);

CREATE OR REPLACE VIEW ploy.runs_timing AS
SELECT
  r.id,
  (EXTRACT(EPOCH FROM (r.started_at - r.created_at)) * 1000)::BIGINT AS queue_ms,
  (EXTRACT(EPOCH FROM (r.finished_at - r.started_at)) * 1000)::BIGINT AS run_ms
FROM ploy.runs r;

DO $$
DECLARE
  missing_job_runs integer;
  missing_run_fields integer;
  orphan_count integer;
BEGIN
  SELECT count(*) INTO missing_job_runs
  FROM ploy.jobs j
  LEFT JOIN ploy.runs r ON r.id = j.run_id
  WHERE r.id IS NULL;
  IF missing_job_runs <> 0 THEN
    RAISE EXCEPTION 'wave migration postcheck failed: % jobs without runs', missing_job_runs;
  END IF;

  SELECT count(*) INTO missing_run_fields
  FROM ploy.runs
  WHERE wave_id IS NULL OR repo_id IS NULL OR source_commit_sha = '';
  IF missing_run_fields <> 0 THEN
    RAISE EXCEPTION 'wave migration postcheck failed: % incomplete runs', missing_run_fields;
  END IF;

  SELECT count(*) INTO orphan_count
  FROM ploy.wave_migration_orphan_runs_20260601;
  RAISE NOTICE 'wave migration quarantined % orphan parent runs', orphan_count;
END $$;

COMMIT;
