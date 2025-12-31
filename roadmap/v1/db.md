# Roadmap v1 — DB

## New tables

### `mods`

- `id TEXT PRIMARY KEY` (NanoID(6), app-generated)
- `name TEXT NOT NULL UNIQUE`
- `default_spec_id TEXT NULL` (FK to `mod_specs.id`)
- `created_by TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `archived_at TIMESTAMPTZ NULL`

Indexes:

- unique on `name`
- optional partial index on active mods: `WHERE archived_at IS NULL`

### `mod_specs`

- `id TEXT PRIMARY KEY` (NanoID(6), app-generated)
- `mod_id TEXT NOT NULL REFERENCES mods(id) ON DELETE CASCADE`
- `name TEXT NOT NULL` (unique within mod)
- `spec JSONB NOT NULL` (canonical Mods spec JSON)
- `created_by TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `archived_at TIMESTAMPTZ NULL`

Constraints / indexes:

- `UNIQUE (mod_id, name)`
- index on `(mod_id, created_at)`

### `mod_repos`

- `id TEXT PRIMARY KEY` (app-generated string ID; choose NanoID length during implementation)
- `mod_id TEXT NOT NULL REFERENCES mods(id) ON DELETE CASCADE`
- `repo_url TEXT NOT NULL`
- `base_ref TEXT NOT NULL`
- `target_ref TEXT NOT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Constraints / indexes:

- `UNIQUE (mod_id, repo_url)`
- index on `(mod_id, created_at)`

Deletion semantics:

- Default CLI behavior: refuse to delete a mod repo if any executions reference it.
  - “Referenced” means at least one `run_repos` row has `mod_repo_id = mod_repos.id`.
- `--force`: allow deletion; preserve history by defining `run_repos.mod_repo_id` as
  `REFERENCES mod_repos(id) ON DELETE SET NULL`.

## Link existing execution tables

### `runs`

Add nullable columns:

- `mod_id TEXT NULL REFERENCES mods(id) ON DELETE SET NULL`
- `mod_spec_id TEXT NULL REFERENCES mod_specs(id) ON DELETE SET NULL`

Remove column:

- `name` (grouping by `runs.name` is removed; use `mods.name` + `runs.mod_id` instead)

Indexes:

- `(mod_id, created_at)`
- `(mod_spec_id)`

### `run_repos`

Add nullable column:

- `mod_repo_id TEXT NULL REFERENCES mod_repos(id) ON DELETE SET NULL`

Index:

- `(mod_repo_id, created_at)`

## Derived “failed repos” selection

Define “last terminal state” per `mod_repo_id` by looking at the newest `run_repos` row (joined via `runs.mod_id`) where status in `(succeeded, failed, skipped, cancelled)` and selecting those where status=`failed`.

## Notes

- No backward compatibility layers: these tables are the new source of truth for mod/spec/repo management.
- Runs not created via a mod remain valid; their `mod_id/mod_spec_id/mod_repo_id` columns are NULL.
