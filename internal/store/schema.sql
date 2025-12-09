-- internal/store/schema.sql — Postgres schema outline for the simplified Ploy server
-- Notes
-- - Uses pgcrypto for UUID generation via gen_random_uuid().
-- - Stores only metadata and run artifacts (diffs/logs/events). No repository
--   contents are ever stored on the server; nodes fetch repos directly by URL.

CREATE SCHEMA IF NOT EXISTS ploy;
SET search_path TO ploy, public;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Enums
CREATE TYPE run_status AS ENUM (
  'queued', 'assigned', 'running', 'succeeded', 'failed', 'canceled'
);

CREATE TYPE job_status AS ENUM (
  'created', 'pending', 'running', 'succeeded', 'failed', 'skipped', 'canceled'
);

-- RunRepoStatus tracks per-repo execution state within a batched run.
-- Mirrors job_status without 'created' since repos enter as 'pending'.
CREATE TYPE run_repo_status AS ENUM (
  'pending', 'running', 'succeeded', 'failed', 'skipped', 'cancelled'
);



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

-- Runs (acts as a queue with SKIP LOCKED assignment)
-- The `name` column provides an optional human-readable batch name for grouping
-- or identifying runs; when NULL, the run is unnamed (single-repo or ad-hoc).
-- Note: id is TEXT (KSUID-backed) rather than UUID; application code generates IDs
-- via types.NewRunID() before insertion.
CREATE TABLE IF NOT EXISTS runs (
  id           TEXT PRIMARY KEY,  -- KSUID-backed string ID (27 chars); no default, app-generated.
  name         TEXT,  -- Optional batch name for human-readable identification.
  repo_url     TEXT NOT NULL,
  spec         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by   TEXT,
  status       run_status NOT NULL DEFAULT 'queued',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,  -- NanoID string FK to nodes.id.
  base_ref     TEXT NOT NULL,
  target_ref   TEXT NOT NULL,
  commit_sha   TEXT,
  stats        JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS runs_status_idx ON runs(status);
CREATE INDEX IF NOT EXISTS runs_node_idx ON runs(node_id);
CREATE INDEX IF NOT EXISTS runs_created_idx ON runs(created_at);

-- RunRepos tracks per-repo execution state within a batched run.
-- The parent run holds shared spec and metadata; each run_repos row captures
-- a single repository's execution state, allowing multiple repos per batch.
-- execution_run_id links to the child run created for this repo's job pipeline.
-- Note: id is TEXT (NanoID-backed, 8 chars) for compact, human-friendly IDs.
-- Note: run_id and execution_run_id are TEXT (KSUID-backed) to match runs.id.
CREATE TABLE IF NOT EXISTS run_repos (
  id               TEXT PRIMARY KEY,  -- NanoID-backed string ID (8 chars); no default, app-generated via NewRunRepoID().
  run_id           TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  repo_url         TEXT NOT NULL,
  base_ref         TEXT NOT NULL,
  target_ref       TEXT NOT NULL,
  status           run_repo_status NOT NULL DEFAULT 'pending',
  attempt          INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  last_error       TEXT,
  execution_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,  -- Child run for this repo's execution; KSUID string.
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at       TIMESTAMPTZ,
  finished_at      TIMESTAMPTZ
);
-- Index for listing repos by run (batch lookups).
CREATE INDEX IF NOT EXISTS run_repos_run_idx ON run_repos(run_id);
-- Partial index for scheduling: find pending/running repos efficiently.
CREATE INDEX IF NOT EXISTS run_repos_status_idx ON run_repos(status) WHERE status IN ('pending','running');
-- Index for finding run_repos by execution_run_id (for completion callbacks).
CREATE INDEX IF NOT EXISTS run_repos_execution_run_idx ON run_repos(execution_run_id) WHERE execution_run_id IS NOT NULL;

-- Jobs (unified job queue for all execution units: pre-gate, mod, heal, post-gate, gate, build)
-- Float step_index enables inserting healing jobs between existing jobs:
--   pre-gate=1000, mod=2000, post-gate=3000
--   heal-1 inserted at 1500, re-gate at 1750, etc.
-- Server-driven scheduling: first job is 'pending', rest are 'created'.
-- When a job completes, server schedules the next 'created' job.
-- Note: id is TEXT (KSUID-backed); run_id is TEXT to match runs.id.
--
-- The `meta` column stores structured job metadata as JSONB. The schema is
-- defined by internal/workflow/contracts.JobMeta with the following shape:
--   {
--     "kind": "mod"|"gate"|"build",
--     "gate": { ... BuildGateStageMetadata ... },   // present when kind="gate"
--     "build": { "tool": "...", "command": "...", "metrics": {...} }  // present when kind="build"
--   }
-- See internal/workflow/contracts.BuildGateStageMetadata for gate metadata fields.
-- See internal/workflow/contracts.BuildMeta for build metadata fields.
CREATE TABLE IF NOT EXISTS jobs (
  id           TEXT PRIMARY KEY,  -- KSUID-backed string ID (27 chars); no default, app-generated.
  run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  status       job_status NOT NULL DEFAULT 'created',
  mod_type     TEXT NOT NULL DEFAULT '',
  mod_image    TEXT NOT NULL DEFAULT '',
  step_index   FLOAT NOT NULL DEFAULT 0,  -- float for dynamic insertion between jobs
  node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,  -- NanoID string FK to nodes.id; which node claimed this job.
  exit_code    INT,  -- exit code from job execution (null until completed)
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  duration_ms  BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  meta         JSONB NOT NULL DEFAULT '{}'::jsonb,  -- Structured metadata; see JobMeta type docs above.
  UNIQUE (run_id, name)
);
CREATE INDEX IF NOT EXISTS jobs_run_idx ON jobs(run_id);
CREATE INDEX IF NOT EXISTS jobs_pending_idx ON jobs(run_id, step_index) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS jobs_node_idx ON jobs(node_id) WHERE node_id IS NOT NULL;

-- Events (append-only)
-- Note: run_id and job_id are TEXT (KSUID-backed) to match runs.id and jobs.id.
-- events.id remains BIGSERIAL per ROADMAP.md (unchanged).
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
-- Each execution job (mod, healing, pre_gate, post_gate) may produce a diff.
-- Diffs store `job_id` and `run_id` for association; summary JSONB contains:
--   - mod_type: "mod", "healing", "pre_gate", "post_gate" (for filtering)
-- Rehydration applies diffs from jobs ordered by step_index.
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
CREATE TABLE IF NOT EXISTS diffs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  patch      BYTEA NOT NULL CHECK (octet_length(patch) <= 1048576),      -- expected gzipped (cap: 1 MiB)
  summary    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS diffs_run_idx ON diffs(run_id);
CREATE INDEX IF NOT EXISTS diffs_job_idx ON diffs(job_id);

-- Logs (append-only)
-- Logs are now grouped at the job level only; build_id column removed as part of
-- the builds table removal (job-level grouping is canonical).
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
-- logs.id remains BIGSERIAL per ROADMAP.md (unchanged).
CREATE TABLE IF NOT EXISTS logs (
  id         BIGSERIAL PRIMARY KEY,
  run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  chunk_no   INTEGER NOT NULL,
  data       BYTEA NOT NULL CHECK (octet_length(data) <= 1048576),      -- expected gzipped (cap: 1 MiB)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique constraint on (run_id, job_id, chunk_no) for job-level log grouping.
CREATE UNIQUE INDEX IF NOT EXISTS logs_run_job_chunk_uniq ON logs(run_id, job_id, chunk_no);
CREATE INDEX IF NOT EXISTS logs_run_idx ON logs(run_id);

-- Artifact bundles (zipped tar of changed files or outputs)
-- Artifact bundles are now grouped at the job level only; build_id column removed as
-- part of the builds table removal (job-level grouping is canonical).
-- Note: run_id and job_id are TEXT (KSUID-backed) to match their parent tables.
-- artifact_bundles.id remains UUID per ROADMAP.md (unchanged).
CREATE TABLE IF NOT EXISTS artifact_bundles (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  job_id     TEXT REFERENCES jobs(id) ON DELETE SET NULL,
  name       TEXT,                -- optional logical name
  bundle     BYTEA NOT NULL CHECK (octet_length(bundle) <= 1048576),      -- expected gzipped tar (cap: 1 MiB)
  cid        TEXT,                -- content-addressed ID for deduplication
  digest     TEXT,                -- content hash
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
-- Per ROADMAP.md: scope controls which job types receive the env var (mods, heal, gate, all).
-- The secret flag indicates whether the value should be redacted at the CLI/HTTP layer.
-- Primary key on 'key' ensures uniqueness; upsert semantics for updates.
CREATE TABLE IF NOT EXISTS config_env (
  key         TEXT PRIMARY KEY,                           -- Environment variable name (e.g., CA_CERTS_PEM_BUNDLE)
  value       TEXT NOT NULL,                              -- Environment variable value (may be large, e.g., PEM bundles)
  scope       TEXT NOT NULL,                              -- Selection scope: 'mods', 'heal', 'gate', 'all'
  secret      BOOLEAN NOT NULL DEFAULT TRUE,              -- If true, value is redacted in list views
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()          -- Last modification timestamp
);
-- Index for listing by scope (useful for job claim filtering).
CREATE INDEX IF NOT EXISTS config_env_scope_idx ON config_env(scope);

-- Advisory lock usage (documentation only)
--   Assignment query sketch:
--   WITH cte AS (
--     SELECT id FROM runs
--     WHERE status = 'queued'
--     ORDER BY created_at
--     FOR UPDATE SKIP LOCKED
--     LIMIT 1
--   )
--   UPDATE runs r SET status='assigned', node_id=$1, started_at=now()
--   FROM cte WHERE r.id = cte.id
--   RETURNING r.*;

-- Optional convenience view for timing
CREATE OR REPLACE VIEW runs_timing AS
SELECT
  r.id,
  (EXTRACT(EPOCH FROM (r.started_at - r.created_at)) * 1000)::BIGINT AS queue_ms,
  (EXTRACT(EPOCH FROM (r.finished_at - r.started_at)) * 1000)::BIGINT AS run_ms
FROM runs r;
