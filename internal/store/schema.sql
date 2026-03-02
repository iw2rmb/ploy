-- internal/store/schema.sql — Postgres schema outline for the simplified Ploy server
-- Notes
-- - Uses pgcrypto for UUID generation via gen_random_uuid().
-- - Stores only metadata and run artifacts (diffs/logs/events). No repository
--   contents are ever stored on the server; nodes fetch repos directly by URL.

CREATE SCHEMA IF NOT EXISTS ploy;
SET search_path TO ploy, public;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Schema version tracking for deterministic migrations.
-- Tracks which migration versions have been applied to this database.
CREATE TABLE IF NOT EXISTS ploy.schema_version (
  version    BIGINT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL
);

-- Enums
--
-- Status model (see docs/migs-lifecycle.md § "State machines"):
-- - run_status: Started | Cancelled | Finished
-- - run_repo_status: Queued | Running | Cancelled | Fail | Success
-- - job_status: Created | Queued | Running | Success | Fail | Cancelled
--
-- Capitalized values are canonical; no aliases.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = 'run_status' AND n.nspname = 'ploy'
  ) THEN
    CREATE TYPE run_status AS ENUM ('Started', 'Cancelled', 'Finished');
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = 'job_status' AND n.nspname = 'ploy'
  ) THEN
    CREATE TYPE job_status AS ENUM ('Created', 'Queued', 'Running', 'Success', 'Fail', 'Cancelled');
  END IF;
END $$;

-- RunRepoStatus tracks per-repo execution state within a batched run.
-- Status values: Queued, Running, Cancelled, Fail, Success.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = 'run_repo_status' AND n.nspname = 'ploy'
  ) THEN
    CREATE TYPE run_repo_status AS ENUM ('Queued', 'Running', 'Cancelled', 'Fail', 'Success');
  END IF;
END $$;

-- PrepGate lifecycle was removed. Drop legacy prep status/runs objects.
DROP TABLE IF EXISTS prep_runs;
DROP TYPE IF EXISTS prep_status;



-- Nodes (no labels; each node must have an IP address).
-- Note: id is TEXT (NanoID-backed, 6 chars) for compact, human-friendly node identifiers.
-- Application code generates IDs via types.NewNodeKey() before insertion.
CREATE TABLE IF NOT EXISTS nodes (
  id              TEXT PRIMARY KEY,  -- NanoID-backed string ID (6 chars); no default, app-generated via NewNodeKey().
  name            TEXT NOT NULL,
  ip_address      INET NOT NULL,
  version         TEXT,
  concurrency     INTEGER NOT NULL DEFAULT 1 CHECK (concurrency >= 1),
  -- Snapshot resource metrics updated on heartbeat (no history kept here)
  cpu_total_millis INTEGER NOT NULL DEFAULT 0 CHECK (cpu_total_millis >= 0),
  cpu_free_millis  INTEGER NOT NULL DEFAULT 0 CHECK (cpu_free_millis >= 0 AND cpu_free_millis <= cpu_total_millis),
  mem_total_bytes  BIGINT  NOT NULL DEFAULT 0 CHECK (mem_total_bytes >= 0),
  mem_free_bytes   BIGINT  NOT NULL DEFAULT 0 CHECK (mem_free_bytes >= 0 AND mem_free_bytes <= mem_total_bytes),
  disk_total_bytes BIGINT  NOT NULL DEFAULT 0 CHECK (disk_total_bytes >= 0),
  disk_free_bytes  BIGINT  NOT NULL DEFAULT 0 CHECK (disk_free_bytes >= 0 AND disk_free_bytes <= disk_total_bytes),
  -- Node rollout control: when true, scheduler must not assign new runs
  drained          BOOLEAN NOT NULL DEFAULT false,
  -- mTLS certificate metadata for audit/rotation
  cert_serial       TEXT,
  cert_fingerprint  TEXT,
  cert_not_before   TIMESTAMPTZ,
  cert_not_after    TIMESTAMPTZ,
  last_heartbeat  TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (name),
  UNIQUE (ip_address)
);
CREATE INDEX IF NOT EXISTS nodes_last_heartbeat_idx ON nodes(last_heartbeat);
-- Query non-drained nodes efficiently for claim paths
CREATE INDEX IF NOT EXISTS nodes_drained_idx ON nodes(drained) WHERE NOT drained;
-- One server responds for one cluster only; nodes implicitly belong to this server's cluster.

-- Specs (dictionary of all Migs specs; append-only)
-- Migs do not "own" specs; a mig just points at a single current spec via migs.spec_id.
-- Setting/updating a mig spec means: insert a new specs row and update migs.spec_id.
-- Note: id is TEXT (NanoID-backed, 8 chars) for stable run references over time.
-- Application code generates IDs via types.NewSpecID() before insertion.
CREATE TABLE IF NOT EXISTS specs (
  id           TEXT PRIMARY KEY,  -- NanoID-backed string ID (8 chars); no default, app-generated via NewSpecID().
  name         TEXT NOT NULL DEFAULT '',  -- Optional human label.
  spec         JSONB NOT NULL,  -- Canonical Migs spec JSON.
  created_by   TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at  TIMESTAMPTZ NULL  -- Optional archiving support; not currently enforced.
);
CREATE INDEX IF NOT EXISTS specs_created_idx ON specs(created_at);

-- Migs (code modification projects)
-- A mig is a long-lived project with a unique name that references a spec and manages a repo set.
-- Note: id is TEXT (NanoID-backed, 6 chars) for compact, human-friendly mig identifiers.
-- Application code generates IDs via types.NewMigID() before insertion.
CREATE TABLE IF NOT EXISTS migs (
  id           TEXT PRIMARY KEY,  -- NanoID-backed string ID (6 chars); no default, app-generated via NewMigID().
  name         TEXT NOT NULL UNIQUE,  -- Human-readable unique name for the mig project.
  spec_id      TEXT REFERENCES specs(id) ON DELETE SET NULL,  -- Current spec; NULL if no spec set yet.
  created_by   TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at  TIMESTAMPTZ NULL  -- When non-NULL, creating new runs for this mig must fail.
);
CREATE INDEX IF NOT EXISTS migs_name_idx ON migs(name);
-- Optional partial index on active migs for efficient filtering.
CREATE INDEX IF NOT EXISTS migs_active_idx ON migs(id) WHERE archived_at IS NULL;

-- Repos (global repository identity, independent from mig membership).
-- Note: id is TEXT (NanoID-backed, 8 chars); application code generates IDs.
CREATE TABLE IF NOT EXISTS repos (
  id           TEXT PRIMARY KEY,
  url          TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (url)
);
CREATE INDEX IF NOT EXISTS repos_created_idx ON repos(created_at);

-- Build stacks catalog (seeded from gates/stacks.yaml).
CREATE TABLE IF NOT EXISTS stacks (
  id           BIGSERIAL PRIMARY KEY,
  lang         TEXT NOT NULL,
  release      TEXT NOT NULL,
  tool         TEXT,
  image        TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (lang, release, tool)
);
CREATE INDEX IF NOT EXISTS stacks_lang_release_idx ON stacks(lang, release);

-- Gate profiles indexed by exact execution identity (repo_id + repo_sha + stack_id).
-- Default profiles use NULL repo_id/repo_sha and are stack-scoped only.
CREATE TABLE IF NOT EXISTS gate_profiles (
  id           BIGSERIAL PRIMARY KEY,
  repo_id      TEXT REFERENCES repos(id) ON DELETE CASCADE,
  repo_sha     TEXT,
  repo_sha8    TEXT,
  stack_id     BIGINT NOT NULL REFERENCES stacks(id) ON DELETE RESTRICT,
  url          TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (repo_id, repo_sha, stack_id)
);
CREATE INDEX IF NOT EXISTS gate_profiles_stack_updated_idx ON gate_profiles(stack_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS gate_profiles_repo_stack_updated_idx ON gate_profiles(repo_id, stack_id, updated_at DESC, id DESC);

-- ModRepos (managed repo set for a mig project)
-- Each row represents a repo participating in a mig, with mutable refs.
-- Note: id is TEXT (NanoID-backed, 8 chars) for compact, human-friendly repo identifiers.
-- Application code generates IDs via types.NewMigRepoID() before insertion.
CREATE TABLE IF NOT EXISTS mig_repos (
  id           TEXT PRIMARY KEY,  -- NanoID-backed string ID (8 chars); no default, app-generated via NewMigRepoID().
  mig_id       TEXT NOT NULL REFERENCES migs(id) ON DELETE CASCADE,
  repo_id      TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,
  base_ref     TEXT NOT NULL,  -- Mutable base ref (e.g., main).
  target_ref   TEXT NOT NULL,  -- Mutable target ref (e.g., feature-branch).
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE IF EXISTS mig_repos
  ADD COLUMN IF NOT EXISTS repo_id TEXT;
ALTER TABLE IF EXISTS mig_repos
  DROP COLUMN IF EXISTS repo_url,
  DROP COLUMN IF EXISTS gate_profile_updated_at,
  DROP COLUMN IF EXISTS gate_profile,
  DROP COLUMN IF EXISTS gate_profile_artifacts,
  DROP COLUMN IF EXISTS prep_updated_at,
  DROP COLUMN IF EXISTS prep_profile,
  DROP COLUMN IF EXISTS prep_artifacts,
  DROP COLUMN IF EXISTS prep_status,
  DROP COLUMN IF EXISTS prep_attempts,
  DROP COLUMN IF EXISTS prep_last_error,
  DROP COLUMN IF EXISTS prep_failure_code;
ALTER TABLE IF EXISTS mig_repos
  ALTER COLUMN repo_id SET NOT NULL;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = 'ploy'
      AND t.relname = 'mig_repos'
      AND c.conname = 'mig_repos_repo_id_fkey'
  ) THEN
    ALTER TABLE ploy.mig_repos
      ADD CONSTRAINT mig_repos_repo_id_fkey
      FOREIGN KEY (repo_id) REFERENCES ploy.repos(id) ON DELETE RESTRICT;
  END IF;
END $$;

DROP INDEX IF EXISTS mig_repos_mig_repo_uniq;
-- Enforce uniqueness: one repo membership per mig.
CREATE UNIQUE INDEX IF NOT EXISTS mig_repos_mig_repo_uniq ON mig_repos(mig_id, repo_id);
CREATE INDEX IF NOT EXISTS mig_repos_mig_created_idx ON mig_repos(mig_id, created_at);

-- Runs (execution of one spec_id over a specific set of repos)
-- v1 model: A run represents the execution of a mig's spec over its repo set.
-- No repo-level fields here; repo attribution comes from run_repos and jobs.repo_id.
-- Note: id is TEXT (KSUID-backed) rather than UUID; application code generates IDs
-- via types.NewRunID() before insertion.
CREATE TABLE IF NOT EXISTS runs (
  id           TEXT PRIMARY KEY,  -- KSUID-backed string ID (27 chars); no default, app-generated.
  mig_id       TEXT NOT NULL REFERENCES migs(id) ON DELETE RESTRICT,  -- Mig project this run belongs to.
  spec_id      TEXT NOT NULL REFERENCES specs(id) ON DELETE RESTRICT,  -- Spec used for this run (immutable snapshot).
  created_by   TEXT,
  status       run_status NOT NULL DEFAULT 'Started',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,  -- Set on run creation.
  finished_at  TIMESTAMPTZ,  -- Set when status transitions to terminal (Finished or Cancelled).
  stats        JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS runs_status_idx ON runs(status);
CREATE INDEX IF NOT EXISTS runs_created_idx ON runs(created_at);
CREATE INDEX IF NOT EXISTS runs_mig_idx ON runs(mig_id);

-- RunRepos tracks per-repo execution state within a run.
-- v1 model: composite PK (run_id, repo_id); execution_run_id removed (no child runs per repo).
-- repo_base_ref and repo_target_ref are snapshots copied from mig_repos at run creation time.
-- Note: run_id is TEXT (KSUID-backed) to match runs.id.
-- Note: repo_id is TEXT (NanoID-backed, 8 chars) to match repos.id.
CREATE TABLE IF NOT EXISTS run_repos (
  mig_id           TEXT NOT NULL REFERENCES migs(id) ON DELETE RESTRICT,  -- Copied from runs.mig_id.
  run_id           TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  repo_id          TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,  -- FK to repos.id.
  repo_base_ref    TEXT NOT NULL,  -- Snapshot of mig_repos.base_ref at run creation time.
  repo_target_ref  TEXT NOT NULL,  -- Snapshot of mig_repos.target_ref at run creation time.
  source_commit_sha TEXT NOT NULL DEFAULT '',  -- Immutable source commit resolved at run start.
  repo_sha0        TEXT NOT NULL DEFAULT '',   -- Initial SHA seed for deterministic job SHA chain.
  status           run_repo_status NOT NULL DEFAULT 'Queued',
  attempt          INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  last_error       TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at       TIMESTAMPTZ,  -- Set when status changes Queued → Running.
  finished_at      TIMESTAMPTZ,  -- Set when status changes to terminal (Fail, Success, Cancelled).
  PRIMARY KEY (run_id, repo_id)  -- Composite PK: one row per repo per run.
);
ALTER TABLE IF EXISTS run_repos
  DROP CONSTRAINT IF EXISTS run_repos_repo_id_fkey;
ALTER TABLE IF EXISTS run_repos
  ADD CONSTRAINT run_repos_repo_id_fkey
  FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS run_repos
  ADD COLUMN IF NOT EXISTS source_commit_sha TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS repo_sha0 TEXT NOT NULL DEFAULT '';
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = 'ploy'
      AND t.relname = 'run_repos'
      AND c.conname = 'run_repos_mig_repo_membership_fkey'
  ) THEN
    ALTER TABLE ploy.run_repos
      ADD CONSTRAINT run_repos_mig_repo_membership_fkey
      FOREIGN KEY (mig_id, repo_id)
      REFERENCES ploy.mig_repos(mig_id, repo_id)
      ON DELETE RESTRICT;
  END IF;
END $$;
-- Index for listing repos by run (batch lookups).
CREATE INDEX IF NOT EXISTS run_repos_run_idx ON run_repos(run_id);
-- Partial index for scheduling: find Queued/Running repos efficiently (v1 status values).
CREATE INDEX IF NOT EXISTS run_repos_status_idx ON run_repos(status) WHERE status IN ('Queued','Running');
-- Index for repo history queries (list runs per repo).
CREATE INDEX IF NOT EXISTS run_repos_repo_created_idx ON run_repos(repo_id, created_at);

-- Jobs (unified job queue for all execution units: pre-build, step, post-build, heal, re-build, mr)
-- Jobs for a repo attempt form a singly-linked chain through next_id -> jobs.id.
-- Server-driven scheduling: only chain head starts as 'Queued'; successors remain 'Created'
-- until their predecessor succeeds and they are promoted.
-- Note: id is TEXT (KSUID-backed); run_id is TEXT to match runs.id.
-- Note: repo_id is TEXT (NanoID-backed, 8 chars) to match repos.id.
--
-- The `meta` column stores structured job metadata as JSONB. The schema is
-- defined by internal/workflow/contracts.JobMeta with the following shape:
--   {
--     "kind": "mig"|"gate"|"build",
--     "gate": { ... BuildGateStageMetadata ... },   // present when kind="gate"
--     "build": { "tool": "...", "command": "...", "metrics": {...} }  // present when kind="build"
--   }
-- See internal/workflow/contracts.BuildGateStageMetadata for gate metadata fields.
-- See internal/workflow/contracts.BuildMeta for build metadata fields.
CREATE TABLE IF NOT EXISTS jobs (
  id              TEXT PRIMARY KEY,  -- KSUID-backed string ID (27 chars); no default, app-generated.
  run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  repo_id         TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,  -- FK to repos.id for repo attribution.
  repo_base_ref   TEXT NOT NULL,  -- Copied from run_repos.repo_base_ref at job creation time.
  attempt         INTEGER NOT NULL,  -- Copied from run_repos.attempt at job creation time.
  name            TEXT NOT NULL,
  status          job_status NOT NULL DEFAULT 'Created',  -- v1: 'Created' or 'Queued' (first job per repo attempt).
  job_type        TEXT NOT NULL DEFAULT '',
  job_image       TEXT NOT NULL DEFAULT '',
  next_id         TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  node_id         TEXT REFERENCES nodes(id) ON DELETE SET NULL,  -- NanoID string FK to nodes.id; which node claimed this job.
  exit_code       INT,  -- exit code from job execution (null until completed)
  started_at      TIMESTAMPTZ,
  finished_at     TIMESTAMPTZ,
  duration_ms     BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  repo_sha_in     TEXT NOT NULL DEFAULT '',   -- Immutable input SHA for this job execution.
  repo_sha_out    TEXT NOT NULL DEFAULT '',   -- Node-reported output SHA after this job.
  repo_sha_in8    TEXT NOT NULL DEFAULT '',   -- Short form of repo_sha_in.
  repo_sha_out8   TEXT NOT NULL DEFAULT '',   -- Short form of repo_sha_out.
  meta            JSONB NOT NULL DEFAULT '{}'::jsonb,  -- Structured metadata; see JobMeta type docs above.
  UNIQUE (run_id, repo_id, attempt, name)  -- unique job name per repo attempt.
);
ALTER TABLE IF EXISTS jobs
  DROP CONSTRAINT IF EXISTS jobs_repo_id_fkey;
ALTER TABLE IF EXISTS jobs
  ADD CONSTRAINT jobs_repo_id_fkey
  FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS jobs
  ADD COLUMN IF NOT EXISTS repo_sha_in TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS repo_sha_out TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS repo_sha_in8 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS repo_sha_out8 TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS jobs_run_idx ON jobs(run_id);
-- 'Queued' is the claimable job status (jobs transition Created → Queued when ready to claim).
CREATE INDEX IF NOT EXISTS jobs_pending_idx ON jobs(run_id, repo_id, attempt, id) WHERE status = 'Queued';
CREATE INDEX IF NOT EXISTS jobs_node_idx ON jobs(node_id) WHERE node_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS jobs_next_id_idx ON jobs(next_id) WHERE next_id IS NOT NULL;
-- Predecessor lookup used to detect whether a Created job is unblocked for promotion.
CREATE INDEX IF NOT EXISTS jobs_predecessor_lookup_idx ON jobs(run_id, repo_id, attempt, next_id);
-- Index for repo attribution queries (logs/diffs/events join via job_id → jobs.repo_id).
CREATE INDEX IF NOT EXISTS jobs_repo_idx ON jobs(repo_id);

-- Gate executions mapped to resolved gate profile rows.
-- One gate record per job_id.
CREATE TABLE IF NOT EXISTS gates (
  job_id       TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
  profile_id   BIGINT NOT NULL REFERENCES gate_profiles(id) ON DELETE RESTRICT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS gates_profile_idx ON gates(profile_id);

-- Events (append-only)
-- Note: run_id and job_id are TEXT (KSUID-backed) to match runs.id and jobs.id.
-- events.id is BIGSERIAL for monotonic cursor semantics (since-id pagination).
CREATE TABLE IF NOT EXISTS events (
  id        BIGSERIAL PRIMARY KEY,
  run_id    TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id    TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  time      TIMESTAMPTZ NOT NULL DEFAULT now(),
  level     TEXT NOT NULL DEFAULT 'info',
  message   TEXT NOT NULL,
  meta      JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS events_run_time_idx ON events USING BRIN (time) WITH (pages_per_range=64);
CREATE INDEX IF NOT EXISTS events_run_idx ON events(run_id);

-- Diffs (per-run, small count)
-- Each execution job (mig, healing, pre_gate, post_gate) may produce a diff.
-- Diffs store `job_id` and `run_id` for association; summary JSONB may include
-- step metadata for ordering and classification (for example: job_type, next_id).
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
-- Blob data is stored in S3-compatible object storage; object_key is a generated column for deterministic paths.
CREATE TABLE IF NOT EXISTS diffs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  patch_size BIGINT NOT NULL CHECK (patch_size > 0),
  object_key TEXT GENERATED ALWAYS AS (
    'diffs/run/' || run_id || '/diff/' || id::text || '.patch.gz'
  ) STORED,
  summary    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS diffs_run_idx ON diffs(run_id);
CREATE INDEX IF NOT EXISTS diffs_job_idx ON diffs(job_id);

-- Logs (append-only)
-- Logs are now grouped at the job level only; build_id column removed as part of
-- the builds table removal (job-level grouping is canonical).
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
-- logs.id is BIGSERIAL for monotonic cursor semantics (since-id pagination).
-- Blob data is stored in S3-compatible object storage; object_key is a generated column for deterministic paths.
CREATE TABLE IF NOT EXISTS logs (
  id         BIGSERIAL PRIMARY KEY,
  run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  chunk_no   INTEGER NOT NULL,
  data_size  BIGINT NOT NULL CHECK (data_size > 0),
  object_key TEXT GENERATED ALWAYS AS (
    'logs/run/' || run_id || '/job/' || COALESCE(job_id, 'none') || '/chunk/' || chunk_no::text || '/log/' || id::text || '.gz'
  ) STORED,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique constraint on (run_id, job_id, chunk_no) for job-level log grouping.
CREATE UNIQUE INDEX IF NOT EXISTS logs_run_job_chunk_uniq ON logs(run_id, job_id, chunk_no);
CREATE INDEX IF NOT EXISTS logs_run_idx ON logs(run_id);

-- Artifact bundles (zipped tar of changed files or outputs)
-- Artifact bundles are now grouped at the job level only; build_id column removed as
-- part of the builds table removal (job-level grouping is canonical).
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
-- artifact_bundles.id is UUID to allow client-side generation and offline staging.
-- Blob data is stored in S3-compatible object storage; object_key is a generated column for deterministic paths.
CREATE TABLE IF NOT EXISTS artifact_bundles (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id      TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  name        TEXT,                -- optional logical name
  bundle_size BIGINT NOT NULL CHECK (bundle_size > 0),
  object_key  TEXT GENERATED ALWAYS AS (
    'artifacts/run/' || run_id || '/bundle/' || id::text || '.tar.gz'
  ) STORED,
  cid         TEXT,                -- content-addressed ID for deduplication
  digest      TEXT,                -- content hash
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS artifact_bundles_run_idx ON artifact_bundles(run_id);
CREATE INDEX IF NOT EXISTS artifact_bundles_job_idx ON artifact_bundles(job_id);
CREATE INDEX IF NOT EXISTS artifact_bundles_cid_idx ON artifact_bundles(cid) WHERE cid IS NOT NULL;

-- Node metrics history (optional, TTL purged; latest snapshot lives in nodes)
CREATE TABLE IF NOT EXISTS node_metrics (
  id               BIGSERIAL PRIMARY KEY,
  node_id          TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,  -- NanoID string FK to nodes.id.
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  cpu_total_millis INTEGER NOT NULL DEFAULT 0,
  cpu_free_millis  INTEGER NOT NULL DEFAULT 0,
  mem_total_bytes  BIGINT  NOT NULL DEFAULT 0,
  mem_free_bytes   BIGINT  NOT NULL DEFAULT 0,
  disk_total_bytes BIGINT  NOT NULL DEFAULT 0,
  disk_free_bytes  BIGINT  NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS node_metrics_node_time_idx ON node_metrics USING BRIN (created_at);
CREATE INDEX IF NOT EXISTS node_metrics_node_idx ON node_metrics(node_id);

-- Job resource consumption history.
-- One row per job execution (job_id), populated from node-reported completion stats.
CREATE TABLE IF NOT EXISTS job_metrics (
  id                  BIGSERIAL PRIMARY KEY,
  node_id             TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,  -- NanoID string FK to nodes.id.
  job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,   -- KSUID string FK to jobs.id.
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  cpu_consumed_ns     BIGINT NOT NULL DEFAULT 0 CHECK (cpu_consumed_ns >= 0),
  disk_consumed_bytes BIGINT NOT NULL DEFAULT 0 CHECK (disk_consumed_bytes >= 0),
  mem_consumed_bytes  BIGINT NOT NULL DEFAULT 0 CHECK (mem_consumed_bytes >= 0),
  UNIQUE (job_id)
);
CREATE INDEX IF NOT EXISTS job_metrics_node_time_idx ON job_metrics USING BRIN (created_at);
CREATE INDEX IF NOT EXISTS job_metrics_node_idx ON job_metrics(node_id);

-- API Tokens (bearer tokens for API access)
CREATE TABLE IF NOT EXISTS api_tokens (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash   TEXT NOT NULL UNIQUE,
  token_id     TEXT NOT NULL UNIQUE,
  cluster_id   TEXT,
  role         TEXT NOT NULL,
  description  TEXT,
  issued_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  revoked_at   TIMESTAMPTZ,
  created_by   TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS api_tokens_token_id_idx ON api_tokens(token_id);

-- Bootstrap Tokens (one-time node enrollment tokens)
CREATE TABLE IF NOT EXISTS bootstrap_tokens (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash     TEXT NOT NULL UNIQUE,
  token_id       TEXT NOT NULL UNIQUE,
  node_id        TEXT REFERENCES nodes(id) ON DELETE CASCADE,  -- NanoID string FK to nodes.id.
  cluster_id     TEXT,
  issued_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at     TIMESTAMPTZ,
  used_at        TIMESTAMPTZ,
  cert_issued_at TIMESTAMPTZ,
  revoked_at     TIMESTAMPTZ,
  issued_by      TEXT
);
CREATE INDEX IF NOT EXISTS bootstrap_tokens_token_id_idx ON bootstrap_tokens(token_id);

-- Global Environment Variables (config_env)
-- Stores global environment entries (including secrets) for injection into jobs.
-- scope controls which job types receive the env var (migs, heal, gate, all).
-- The secret flag indicates whether the value should be redacted at the CLI/HTTP layer.
-- Primary key on 'key' ensures uniqueness; upsert semantics for updates.
CREATE TABLE IF NOT EXISTS config_env (
  key         TEXT PRIMARY KEY,                           -- Environment variable name (e.g., CA_CERTS_PEM_BUNDLE)
  value       TEXT NOT NULL,                              -- Environment variable value (may be large, e.g., PEM bundles)
  scope       TEXT NOT NULL,                              -- Selection scope: 'migs', 'heal', 'gate', 'all'
  secret      BOOLEAN NOT NULL DEFAULT TRUE,              -- If true, value is redacted in list views
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()          -- Last modification timestamp
);
-- Index for listing by scope (useful for job claim filtering).
CREATE INDEX IF NOT EXISTS config_env_scope_idx ON config_env(scope);

-- Advisory lock usage (documentation only)
-- Note: v1 model does not use run-level assignment; runs are created with status='Started'.
-- Jobs are claimed individually at the job level; see ClaimJob query in jobs.sql.

-- Optional convenience view for timing
CREATE OR REPLACE VIEW runs_timing AS
SELECT
  r.id,
  (EXTRACT(EPOCH FROM (r.started_at - r.created_at)) * 1000)::BIGINT AS queue_ms,
  (EXTRACT(EPOCH FROM (r.finished_at - r.started_at)) * 1000)::BIGINT AS run_ms
FROM runs r;
