-- Migration 008: Add run_steps table for step-level execution tracking
-- Purpose: Enable multi-node execution by tracking individual step claims and status.
--
-- Context: Multi-step Mods runs consist of sequential gate+mod steps. Previously,
--          entire runs were claimed atomically by one node. This migration introduces
--          per-step tracking so different nodes can execute distinct steps of the same
--          run, enabling parallel execution and better resource utilization.
--
-- Usage: Each row represents one step of a run. step_index is 0-based and matches the
--        step_index in the diffs table. Steps are created when a multi-step run is queued.
--        For single-step runs (legacy), no rows are created; the run itself is claimed.

-- run_step_status tracks the lifecycle of a single step within a run.
-- States mirror run_status but are scoped to individual steps.
CREATE TYPE run_step_status AS ENUM (
  'queued',    -- Step is waiting to be claimed by a node
  'assigned',  -- Step has been claimed by a node but not yet started
  'running',   -- Step is currently executing on a node
  'succeeded', -- Step completed successfully
  'failed',    -- Step failed during execution
  'canceled'   -- Step was canceled before or during execution
);

-- run_steps tracks execution state for each step of a multi-step run.
-- For single-step runs (legacy), this table is empty and the run is claimed atomically.
-- For multi-step runs, each step is represented as a row and can be claimed independently.
CREATE TABLE IF NOT EXISTS run_steps (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id       UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  step_index   INTEGER NOT NULL CHECK (step_index >= 0),
  status       run_step_status NOT NULL DEFAULT 'queued',
  node_id      UUID REFERENCES nodes(id) ON DELETE SET NULL,
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  reason       TEXT,
  UNIQUE (run_id, step_index)
);

-- Index for efficient step claiming queries (WHERE status = 'queued' ORDER BY run_id, step_index).
CREATE INDEX run_steps_status_idx ON run_steps(status, run_id, step_index);

-- Index for listing steps by run (common query pattern).
CREATE INDEX run_steps_run_idx ON run_steps(run_id, step_index);

-- Index for listing steps by node (for node status queries).
CREATE INDEX run_steps_node_idx ON run_steps(node_id);

-- Comment: The claim strategy for multi-step runs uses FOR UPDATE SKIP LOCKED on run_steps
--          to assign individual steps. The scheduler must ensure step k-1 has succeeded
--          before allowing step k to be claimed, maintaining sequential execution semantics
--          while enabling different nodes to execute different steps.
