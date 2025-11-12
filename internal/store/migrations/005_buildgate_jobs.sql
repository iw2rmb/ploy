-- Migration: Add buildgate_jobs table for queue-based build validation
SET search_path TO ploy, public;

-- Enum for build gate job status
CREATE TYPE buildgate_job_status AS ENUM (
  'pending', 'claimed', 'running', 'completed', 'failed'
);

-- Build gate jobs (queue-based validation requests)
CREATE TABLE IF NOT EXISTS buildgate_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  status          buildgate_job_status NOT NULL DEFAULT 'pending',
  request_payload JSONB NOT NULL,
  result          JSONB,
  error           TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at      TIMESTAMPTZ,
  finished_at     TIMESTAMPTZ,
  node_id         UUID REFERENCES nodes(id) ON DELETE SET NULL
);

-- Index for claiming pending jobs (ordered by creation time)
CREATE INDEX IF NOT EXISTS buildgate_jobs_status_created_idx
  ON buildgate_jobs(status, created_at) WHERE status = 'pending';

-- Index for looking up jobs by ID
CREATE INDEX IF NOT EXISTS buildgate_jobs_node_idx
  ON buildgate_jobs(node_id);

-- Index for querying job status
CREATE INDEX IF NOT EXISTS buildgate_jobs_created_idx
  ON buildgate_jobs(created_at);
