-- SCHEMA.sql — Postgres schema outline for the simplified Ploy server
-- Notes
-- - Uses pgcrypto for UUID generation via gen_random_uuid().
-- - Stores only metadata and run artifacts (diffs/logs/events). No repository
--   contents are ever stored on the server; nodes fetch repos directly by URL.
-- - Partitioning stubs are included for large append-only tables.

CREATE SCHEMA IF NOT EXISTS ploy;
SET search_path TO ploy, public;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Enums
CREATE TYPE run_status AS ENUM (
  'queued', 'assigned', 'running', 'succeeded', 'failed', 'canceled'
);

CREATE TYPE stage_status AS ENUM (
  'pending', 'running', 'succeeded', 'failed', 'skipped', 'canceled'
);

-- Cluster (singleton; exactly one row expected)
CREATE TABLE IF NOT EXISTS cluster (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Nodes (no labels; each node must have an IP address).
CREATE TABLE IF NOT EXISTS nodes (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE TABLE IF NOT EXISTS runs (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  repo_url     TEXT NOT NULL,
  spec         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by   TEXT,
  status       run_status NOT NULL DEFAULT 'queued',
  reason       TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  node_id      UUID REFERENCES nodes(id) ON DELETE SET NULL,
  base_ref     TEXT NOT NULL,
  target_ref   TEXT NOT NULL,
  commit_sha   TEXT,
  stats        JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS runs_status_idx ON runs(status);
CREATE INDEX IF NOT EXISTS runs_node_idx ON runs(node_id);
CREATE INDEX IF NOT EXISTS runs_created_idx ON runs(created_at);

-- Stages
CREATE TABLE IF NOT EXISTS stages (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id       UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  status       stage_status NOT NULL DEFAULT 'pending',
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  duration_ms  BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  meta         JSONB NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE (run_id, name)
);
CREATE INDEX IF NOT EXISTS stages_run_idx ON stages(run_id);

-- Events (append-only)
CREATE TABLE IF NOT EXISTS events (
  id        BIGSERIAL PRIMARY KEY,
  run_id    UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage_id  UUID REFERENCES stages(id) ON DELETE SET NULL,
  time      TIMESTAMPTZ NOT NULL DEFAULT now(),
  level     TEXT NOT NULL DEFAULT 'info',
  message   TEXT NOT NULL,
  meta      JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS events_run_time_idx ON events USING BRIN (time) WITH (pages_per_range=64);
CREATE INDEX IF NOT EXISTS events_run_idx ON events(run_id);

-- Builds (timed invocations inside a stage, e.g., Maven/Gradle/Bazel)
CREATE TABLE IF NOT EXISTS builds (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id       UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage_id     UUID REFERENCES stages(id) ON DELETE SET NULL,
  tool         TEXT,             -- e.g., 'maven', 'gradle', 'npm', 'bazel'
  command      TEXT,             -- full command line if available
  status       stage_status NOT NULL DEFAULT 'pending',
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  duration_ms  BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  metrics      JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS builds_run_idx ON builds(run_id);
CREATE INDEX IF NOT EXISTS builds_stage_idx ON builds(stage_id);

-- Diffs (per-run, small count)
CREATE TABLE IF NOT EXISTS diffs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage_id   UUID REFERENCES stages(id) ON DELETE SET NULL,
  patch      BYTEA NOT NULL CHECK (octet_length(patch) <= 1048576),      -- expected gzipped (cap: 1 MiB)
  summary    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS diffs_run_idx ON diffs(run_id);

-- Logs (append-only)
CREATE TABLE IF NOT EXISTS logs (
  id         BIGSERIAL PRIMARY KEY,
  run_id     UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage_id   UUID REFERENCES stages(id) ON DELETE SET NULL,
  build_id   UUID REFERENCES builds(id) ON DELETE SET NULL,
  chunk_no   INTEGER NOT NULL,
  data       BYTEA NOT NULL CHECK (octet_length(data) <= 1048576),      -- expected gzipped (cap: 1 MiB)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS logs_run_stage_build_chunk_uniq ON logs(run_id, stage_id, build_id, chunk_no);
CREATE INDEX IF NOT EXISTS logs_run_idx ON logs(run_id);

-- Artifact bundles (zipped tar of changed files or outputs)
CREATE TABLE IF NOT EXISTS artifact_bundles (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage_id   UUID REFERENCES stages(id) ON DELETE SET NULL,
  build_id   UUID REFERENCES builds(id) ON DELETE SET NULL,
  name       TEXT,                -- optional logical name
  bundle     BYTEA NOT NULL CHECK (octet_length(bundle) <= 1048576),      -- expected gzipped tar (cap: 1 MiB)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS artifact_bundles_run_idx ON artifact_bundles(run_id);
CREATE INDEX IF NOT EXISTS artifact_bundles_stage_idx ON artifact_bundles(stage_id);

-- Node metrics history (optional, TTL purged; latest snapshot lives in nodes)
CREATE TABLE IF NOT EXISTS node_metrics (
  id               BIGSERIAL PRIMARY KEY,
  node_id          UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
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
