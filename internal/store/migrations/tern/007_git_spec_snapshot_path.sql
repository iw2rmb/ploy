-- Write immutable Git-backed spec snapshots per repository-relative source path.
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

---- create above / drop below ----

DROP INDEX IF EXISTS ploy.specs_named_source_sha_name_idx;

CREATE UNIQUE INDEX specs_named_source_sha_name_idx
ON ploy.specs (name, (source->>'domain'), (source->>'repo'), sha)
WHERE name <> '' AND sha <> '';
