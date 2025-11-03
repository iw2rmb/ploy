-- Add drained flag for nodes to support node draining during rollouts.
-- This column tracks whether a node has been administratively drained and
-- should not claim new run assignments while drained.

SET search_path TO ploy, public;

-- Add drained column with default false.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS drained BOOLEAN NOT NULL DEFAULT false;

-- Create index to efficiently query non-drained nodes during claim operations.
CREATE INDEX IF NOT EXISTS nodes_drained_idx ON nodes(drained) WHERE NOT drained;
