DROP INDEX IF EXISTS ploy.specs_named_source_sha_name_idx;

ALTER TABLE IF EXISTS ploy.specs
  DROP COLUMN IF EXISTS updated_by,
  DROP COLUMN IF EXISTS archived_at;

---- create above / drop below ----

ALTER TABLE IF EXISTS ploy.specs
  ADD COLUMN IF NOT EXISTS updated_by TEXT,
  ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ NULL;

CREATE UNIQUE INDEX IF NOT EXISTS specs_named_source_sha_name_idx
ON ploy.specs (name, (source->>'domain'), (source->>'repo'), sha)
WHERE name <> '' AND sha <> '' AND COALESCE(source->>'path', '') = '';
