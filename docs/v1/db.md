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

Notes:

- `specs` exists to support stable `runs.spec_id` references over time.
- This is not a “spec sharing” feature between mods.

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

Notes:

- `mod_repos.base_ref` and `mod_repos.target_ref` are mutable (e.g., `repo import` updates refs in-place).
- Run history remains stable because `run_repos.repo_base_ref` snapshots the base ref at run creation time.

## Updated execution tables

## Enums (v1)

### `runs.status` (`run_status`)

- `Started`
- `Stopped`
- `Finished`

### `run_repos.status` (`run_repo_status`)

- `Pending`
- `Running`
- `Stopped`
- `Failed`
- `Success`

### `runs`

Run is the execution of one `spec_id` over a specific set of repos.

- `id TEXT PRIMARY KEY` (KSUID(27), app-generated; same as current `runs.id`)
- `mod_id TEXT NOT NULL REFERENCES mods(id) ON DELETE RESTRICT`
- `spec_id TEXT NOT NULL REFERENCES specs(id) ON DELETE RESTRICT` (copied from `mods.spec_id` at creation time)
- `created_by TEXT NULL`
- `status run_status NOT NULL DEFAULT 'Started'`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`
- `stats JSONB NOT NULL DEFAULT '{}'::jsonb`

Notes:

- No `node_id`: jobs are balanced across nodes; run is not “owned” by a node.

### `run_repos`

Per-repo execution state within a run.

- `mod_id TEXT NOT NULL REFERENCES mods(id) ON DELETE RESTRICT` (copied from `runs.mod_id`)
- `run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE`
- `repo_id TEXT NOT NULL REFERENCES mod_repos(id) ON DELETE RESTRICT`
- `repo_base_ref TEXT NOT NULL` (copied from `mod_repos.base_ref` at creation time)
- `status run_repo_status NOT NULL DEFAULT 'Pending'`
- `attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1)`
- `last_error TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`

Constraints / indexes:

- `PRIMARY KEY (run_id, repo_id)`
- index on `(run_id)`
- partial index on `status` for scheduling: `WHERE status IN ('Pending','Running')`
- index on `(repo_id, created_at)`

### `jobs` (updated)

Job rows must be repo-scoped so logs/diffs/events for a run can be attributed to a repo.

- Add:
  - `repo_id TEXT NOT NULL REFERENCES mod_repos(id) ON DELETE RESTRICT`
  - `repo_base_ref TEXT NOT NULL` (copied from `run_repos.repo_base_ref` at job creation time)

## Derived “failed repos” selection

Define “last terminal state” per `repo_id` by looking at the newest `run_repos` row where status in `(Stopped, Failed, Success)` and selecting those where status=`Failed`.

## Notes

- No backward compatibility layers: these tables are the new source of truth for mod/spec/repo management.
