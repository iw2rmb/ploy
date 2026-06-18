ALTER TABLE IF EXISTS ploy.api_tokens
  DROP COLUMN IF EXISTS cluster_id;

ALTER TABLE IF EXISTS ploy.bootstrap_tokens
  DROP COLUMN IF EXISTS cluster_id,
  DROP CONSTRAINT IF EXISTS bootstrap_tokens_node_id_fkey;

ALTER TABLE IF EXISTS ploy.mig_repos
  DROP COLUMN IF EXISTS target_ref;

ALTER TABLE IF EXISTS ploy.runs
  ALTER COLUMN source_commit_sha SET DEFAULT '',
  ALTER COLUMN repo_sha0 SET DEFAULT '',
  ALTER COLUMN status SET DEFAULT 'Queued',
  ALTER COLUMN attempt SET DEFAULT 1,
  ALTER COLUMN stats SET DEFAULT '{}'::jsonb;

DO $$
BEGIN
  IF to_regclass('ploy.node_daemon_logs') IS NOT NULL THEN
    DELETE FROM ploy.node_daemon_logs
    WHERE component <> 'node';
  END IF;
END $$;

DO $$
BEGIN
  IF to_regclass('ploy.node_diagnostics') IS NOT NULL THEN
    DELETE FROM ploy.node_diagnostics
    WHERE component <> 'node';
  END IF;
END $$;

ALTER TABLE IF EXISTS ploy.node_diagnostics
  DROP CONSTRAINT IF EXISTS node_diagnostics_component_check,
  ADD CONSTRAINT node_diagnostics_component_check CHECK (component IN ('node'));

ALTER TABLE IF EXISTS ploy.node_daemon_logs
  DROP CONSTRAINT IF EXISTS node_daemon_logs_component_check,
  ADD CONSTRAINT node_daemon_logs_component_check CHECK (component IN ('node'));

DROP TABLE IF EXISTS ploy.schema_version;
