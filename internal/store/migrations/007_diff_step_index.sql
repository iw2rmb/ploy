-- Migration 007: Add step_index to diffs table
-- Purpose: Attach step identity to stored diffs to enable rehydration logic
--          that can select "all diffs before step k" for multi-step Mods runs.
--
-- Context: Multi-step Mods runs capture one diff per gate+mod step. This column
--          allows ordering diffs by logical step order (gate→mod pairs) instead
--          of relying solely on created_at timestamps, which may not perfectly
--          reflect step sequence due to concurrent operations or clock drift.
--
-- Usage: step_index is 0-based; step 0 is the first gate+mod, step 1 is second, etc.
--        NULL step_index is permitted for legacy diffs or final MR diffs that span
--        multiple steps, ensuring backward compatibility with single-step runs.

-- Add step_index column to diffs table (nullable for backward compatibility).
ALTER TABLE diffs
ADD COLUMN step_index INTEGER CHECK (step_index >= 0);

-- Create index on (run_id, step_index) to efficiently query diffs for specific
-- steps or ranges of steps within a run. Supports queries like "all diffs before step k".
CREATE INDEX diffs_run_step_idx ON diffs(run_id, step_index);

-- Comment: The ordering strategy for ListDiffsByRun should prioritize step_index
--          when present, falling back to created_at for legacy diffs. New queries
--          can filter by step_index to select diffs for rehydration up to a given step.
