-- Preserve legacy publication uniqueness while allowing one immutable Git
-- snapshot for each canonical override result at the same source commit.
DROP INDEX IF EXISTS ploy.specs_named_source_sha_name_idx;

CREATE UNIQUE INDEX specs_named_source_sha_name_idx
ON ploy.specs (name, (source->>'domain'), (source->>'repo'), sha)
WHERE name <> '' AND sha <> '' AND COALESCE(source->>'path', '') = '';

CREATE INDEX specs_git_snapshot_lookup_idx
ON ploy.specs (name, (source->>'domain'), (source->>'repo'), (source->>'path'), sha)
WHERE name <> '' AND sha <> '' AND COALESCE(source->>'path', '') <> '';

---- create above / drop below ----

DROP INDEX IF EXISTS ploy.specs_git_snapshot_lookup_idx;
DROP INDEX IF EXISTS ploy.specs_named_source_sha_name_idx;

CREATE UNIQUE INDEX specs_named_source_sha_name_idx
ON ploy.specs (
  name,
  (source->>'domain'),
  (source->>'repo'),
  (COALESCE(source->>'path', '')),
  sha
)
WHERE name <> '' AND sha <> '';
