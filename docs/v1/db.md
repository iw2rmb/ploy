# Roadmap v1 — DB

## New tables

### `mods`

- `id TEXT PRIMARY KEY` (NanoID(6), app-generated)
- `name TEXT NOT NULL UNIQUE`
- `spec_id TEXT NULL REFERENCES specs(id) ON DELETE SET NULL`
- `created_by TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `archived_at TIMESTAMPTZ NULL`

Indexes:

- unique on `name`
- optional partial index on active mods: `WHERE archived_at IS NULL`

Archiving semantics:

- When `archived_at` is non-NULL, creating new runs for the mod must fail.
- Archiving must be refused when the mod has any jobs in a running state.

### `specs`

Dictionary of all specs uploaded to Spok. Mod “spec variants” are represented by
multiple `specs` rows over time; the mod’s current spec is `mods.spec_id`.

- `id TEXT PRIMARY KEY` (NanoID(8), app-generated)
- `name TEXT NOT NULL`
- `spec JSONB NOT NULL` (canonical Mods spec JSON)
- `created_by TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `archived_at TIMESTAMPTZ NULL`

Constraints / indexes:

- index on `(created_at)`

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

## Updated execution tables

### `runs`

Run is the execution of one `spec_id` over a specific set of repos.

- `id TEXT PRIMARY KEY` (KSUID(27), app-generated; same as current `runs.id`)
- `spec_id TEXT NOT NULL REFERENCES specs(id) ON DELETE RESTRICT` (copied from `mods.spec_id` at creation time)
- `created_by TEXT NULL`
- `status run_status NOT NULL DEFAULT 'queued'`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`
- `stats JSONB NOT NULL DEFAULT '{}'::jsonb`

Notes:

- No `node_id`: jobs are balanced across nodes; run is not “owned” by a node.

### `run_repos`

Per-repo execution state within a run.

- `run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE`
- `repo_id TEXT NOT NULL REFERENCES mod_repos(id) ON DELETE RESTRICT`
- `status run_repo_status NOT NULL DEFAULT 'pending'`
- `attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1)`
- `last_error TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`

Constraints / indexes:

- `PRIMARY KEY (run_id, repo_id)`
- index on `(run_id)`
- partial index on `status` for scheduling: `WHERE status IN ('pending','running')`
- index on `(repo_id, created_at)`

## Derived “failed repos” selection

Define “last terminal state” per `repo_id` by looking at the newest `run_repos` row where status in `(succeeded, failed, skipped, cancelled)` and selecting those where status=`failed`.

## Notes

- No backward compatibility layers: these tables are the new source of truth for mod/spec/repo management.
