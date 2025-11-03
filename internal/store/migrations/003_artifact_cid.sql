-- Migration 003: Add CID and digest columns to artifact_bundles table for content-addressable lookups.

-- Add cid column (content identifier, typically SHA256-based hash string like "bafy...")
ALTER TABLE artifact_bundles
ADD COLUMN cid TEXT;

-- Add digest column (SHA256 hex digest for verification)
ALTER TABLE artifact_bundles
ADD COLUMN digest TEXT;

-- Create index on cid for fast lookups by content identifier
CREATE INDEX IF NOT EXISTS idx_artifact_bundles_cid ON artifact_bundles(cid);

-- Create index on name for artifact listing queries
CREATE INDEX IF NOT EXISTS idx_artifact_bundles_name ON artifact_bundles(name);
