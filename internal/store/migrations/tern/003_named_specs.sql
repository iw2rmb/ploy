ALTER TABLE IF EXISTS ploy.specs
  ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS sha TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_committed_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS ploy.specs
  DROP CONSTRAINT IF EXISTS specs_sha_check,
  ADD CONSTRAINT specs_sha_check CHECK (sha = '' OR sha ~ '^[0-9a-f]{40}$');

ALTER TABLE IF EXISTS ploy.specs
  DROP CONSTRAINT IF EXISTS specs_named_required_check,
  ADD CONSTRAINT specs_named_required_check CHECK (
    sha = '' OR (
      name <> ''
      AND COALESCE(source->>'domain', '') <> ''
      AND COALESCE(source->>'repo', '') <> ''
      AND source_committed_at IS NOT NULL
    )
  );

CREATE UNIQUE INDEX IF NOT EXISTS specs_named_source_sha_name_idx
ON ploy.specs (name, (source->>'domain'), (source->>'repo'), sha)
WHERE name <> '' AND sha <> '';
