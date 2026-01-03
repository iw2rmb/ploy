# Roadmap v1 — DB

## New tables

Change entry: introduce `mods`, `specs`, `mod_repos` for project/spec/repo management.

- Current (HEAD): run-centric schema lives in `internal/store/schema.sql` (`runs`, `run_repos`, `jobs`, `events`, `diffs`, `logs`, `artifact_bundles`).
- Proposed (v1): add `mods`, `specs`, `mod_repos` tables as the new source of truth for mod project state.
- Where: `internal/store/schema.sql` + `internal/store/queries/*` + new CRUD handlers under `internal/server/handlers/*`.
- Compatibility: breaking schema change; no migrations/back-compat required (fresh deploy).
- Unchanged: artifacts/logs/events remain persisted and addressed by `run_id` and `job_id` (repo attribution becomes `job_id → jobs.repo_id`).

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

Dictionary of all specs. Mods do not “own” specs; a mod just points at a single
current spec via `mods.spec_id`.

Notes:

- `specs` exists to support stable `runs.spec_id` references over time.
- There is no v1 concept of “spec variants” as a first-class per-mod set.
  - Setting/updating a mod spec means: insert a new `specs` row and update `mods.spec_id`.
  - Old `specs` rows remain for historical `runs.spec_id` references.

- `id TEXT PRIMARY KEY` (NanoID(8), app-generated)
- `name TEXT NOT NULL DEFAULT ''` (optional human label)
- `spec JSONB NOT NULL` (canonical Mods spec JSON)
- `created_by TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `archived_at TIMESTAMPTZ NULL`

Constraints / indexes:

- index on `(created_at)`

Notes:

- A `specs` row must not be hard-deleted if it is referenced by any `runs.spec_id`.
  v1 does not require any spec-deletion API; treat `specs` as append-only storage.

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
- Run history remains stable because `run_repos.repo_base_ref` and `run_repos.repo_target_ref` snapshot refs at run creation time.

## Updated execution tables

Change entry: reshape execution model to `runs` → `run_repos` and make jobs repo-scoped.

- Current (HEAD): batch execution uses `run_repos.execution_run_id` (child “execution run” per repo) and jobs/diffs/logs/events attach to that child run (see `internal/store/schema.sql`, `internal/store/queries/run_repos.sql`, and scheduling in `internal/server/handlers/runs_batch_scheduler.go`).
- Proposed (v1): remove per-repo execution runs; use a single `runs` row with per-repo `run_repos` rows, and add `jobs.repo_id` + `jobs.repo_base_ref` to attribute artifacts to repos.
- Where: `internal/store/schema.sql` (`runs`, `run_repos`, `jobs`) and refactors in `internal/server/handlers/mods_ticket.go`, `internal/server/handlers/runs_batch_scheduler.go`, `internal/server/handlers/nodes_complete_run.go`.
- Compatibility: breaking DB + scheduling semantics; no backward compatibility required.
- Unchanged: job lifecycle states and ingestion endpoints remain job-addressed (see current `job_status` in `internal/store/schema.sql`).

## Enums (v1)

### `runs.status` (`run_status`)

- `Started`
- `Cancelled`
- `Finished`

### `run_repos.status` (`run_repo_status`)

- `Pending`
- `Running`
- `Cancelled`
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
- `repo_target_ref TEXT NOT NULL` (copied from `mod_repos.target_ref` at creation time)
- `commit_sha TEXT NULL` (recorded base commit SHA for this repo execution; derived from `repo_base_ref` at scheduling time)
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

## Status semantics (v1)

### `runs.status`

- `Started` → `Finished` when all `run_repos` are terminal (`Failed`, `Success`, or `Cancelled`).
- `Started` → `Cancelled` via `ploy run cancel`.

### `run_repos.status`

- Initial status is `Pending`.
- `Running` when there is at least one repo-scoped job with `jobs.status IN ('pending','running')` for `(run_id, repo_id)`.
- Terminal:
  - `Success` when repo execution succeeded.
  - `Failed` when repo execution did not succeed (and was not cancelled).
  - `Cancelled` when repo execution was cancelled (treated as terminal for `runs.status` aggregation).
  - `Cancelled` via repo cancellation endpoint (see `roadmap/v1/api.md`).

### `jobs` (updated)

Job rows must be repo-scoped so logs/diffs/events for a run can be attributed to a repo.

- `id TEXT PRIMARY KEY` (KSUID(27), app-generated; same as current `jobs.id`)
- `run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE`
- `repo_id TEXT NOT NULL REFERENCES mod_repos(id) ON DELETE RESTRICT`
- `repo_base_ref TEXT NOT NULL` (copied from `run_repos.repo_base_ref` at job creation time)
- `name TEXT NOT NULL`
- `status job_status NOT NULL DEFAULT 'created'`
- `mod_type TEXT NOT NULL DEFAULT ''`
- `mod_image TEXT NOT NULL DEFAULT ''`
- `step_index FLOAT NOT NULL DEFAULT 0`
- `node_id TEXT NULL REFERENCES nodes(id) ON DELETE SET NULL`
- `exit_code INT NULL`
- `started_at TIMESTAMPTZ NULL`
- `finished_at TIMESTAMPTZ NULL`
- `duration_ms BIGINT NOT NULL DEFAULT 0`
- `meta JSONB NOT NULL DEFAULT '{}'::jsonb`

Notes:

- Repo attribution for `events` / `diffs` / `logs` / `artifact_bundles` is derived via `job_id → jobs.repo_id`.
- Uniqueness must be per-repo within a run:
  - `UNIQUE (run_id, repo_id, name, step_index)`
- v0 reference: current server-side batch tables use `run_repos.id` as the “repo id” in HTTP paths like `/v1/runs/{id}/repos/{repo_id}`; v1 repurposes `repo_id` to mean `mod_repos.id` (aka `mod_repo_id`).
- v1 rule: `run_repos.commit_sha` is resolved by the server before starting the first job and is not accepted from CLI input.
  - If commit SHA resolution fails, set `run_repos.status = Failed` and populate `run_repos.last_error` (no jobs are started).

### Repo restarts / attempts (TODO decision)

v1 introduces `run_repos.attempt` but the `jobs` uniqueness constraint above does
not allow creating a second set of jobs for the same `(run_id, repo_id)` unless
we also encode attempt into job identity.

Pick one concrete approach before implementation:

- Option A (recommended): add `jobs.attempt INTEGER NOT NULL` copied from `run_repos.attempt` at job creation time and change uniqueness to `UNIQUE (run_id, repo_id, attempt, name, step_index)`.
- Option B: keep `jobs` attempt-less, but on repo restart, hard-delete all repo-scoped jobs (and repo-scoped artifacts) for that `(run_id, repo_id)` before re-creating jobs.

## Derived “failed repos” selection

Define “last terminal state” per `repo_id` by looking at the newest `run_repos` row where status in `(Failed, Success, Cancelled)` and selecting those where status=`Failed`.

## Notes

- No backward compatibility layers: these tables are the new source of truth for mod/spec/repo management.
