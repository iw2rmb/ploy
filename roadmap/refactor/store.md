# Store Refactor Notes (`internal/store`)

## Workspace State

- `git diff` was empty at time of review; `git status` had untracked `roadmap/refactor/*`.

## High-Risk Bugs / Correctness

- Missing `search_path` on app connections.
  - Schema writes everything under `ploy` (`internal/store/schema.sql:7`), but sqlc queries use unqualified table names (e.g. `internal/store/queries/jobs.sql:2`).
  - `internal/store/store.go:28` does not set `RuntimeParams["search_path"]`; correctness depends on DSN including it.
- `RunMigrations` is inconsistent and likely broken for multi-statement schema execution.
  - It claims “supports multi-statement files” but uses `pool.Exec(ctx, schemaSQL)` (`internal/store/migrations.go:48`).
  - `ensureVersionTable` explicitly avoids multi-statement issues (`internal/store/versioning.go:11`), and `execMigrationSQL` exists but is unused (`internal/store/sql_split.go:17`).
  - Migration tests assume `ploy.schema_version` exists and has entries (`internal/store/migrate_test.go:30`), but `internal/store/schema.sql` has no `schema_version` table.
- Job claiming and scheduling are under-specified and race-prone.
  - `ClaimJob` orders globally by `step_index` only (`internal/store/queries/jobs.sql:126`), despite `step_index` being a per-run/per-repo sequencing mechanism (`internal/store/schema.sql:164`).
  - `ClaimJob` uses `FOR UPDATE SKIP LOCKED` with a join to `runs` (`internal/store/queries/jobs.sql:119`); it may lock `runs` rows unnecessarily.
  - `ClaimJob(ctx, nodeID *string)` permits `nil`, which can produce `status='Running'` with `node_id=NULL` (`internal/store/querier.go:25`, `internal/store/queries/jobs.sql:131`).
  - `ScheduleNextJob` selects without `FOR UPDATE SKIP LOCKED` and updates without re-checking status (`internal/store/queries/jobs.sql:175`).
  - `duration_ms` can become NULL if `started_at` is NULL (`internal/store/queries/jobs.sql:218`), violating `jobs.duration_ms BIGINT NOT NULL` (`internal/store/schema.sql:196`).

## Type-System Hardening

- Use sqlc overrides (`sqlc.yaml:1`) to map DB identifiers to domain newtypes instead of `string`:
  - IDs: `runs.id`, `jobs.id`, `mods.id`, `mod_repos.id`, `nodes.id`, `specs.id` are plain `string` (`internal/store/models.go:149`).
- Make `step_index` harder to misuse:
  - DB type is `FLOAT` (`internal/store/schema.sql:191`) and Go type is `float64` (`internal/store/models.go:226`).
  - If floats are kept, wrap in a dedicated type and validate “integer-like” invariants at boundaries.
- Avoid nullable “must-be-present” inputs at the store boundary:
  - `ClaimJob(ctx, nodeID *string)` (`internal/store/querier.go:25`) should be non-nullable (wrapper method or query change).
- Reduce `[]byte` ambiguity for JSONB columns:
  - `Job.Meta`, `Run.Stats`, `Spec.Spec`, `Diff.Summary` are `[]byte` (`internal/store/models.go:201`, `internal/store/models.go:304`, `internal/store/models.go:330`).
  - Prefer `json.RawMessage` or typed structs at API/service boundaries to prevent storing non-JSON bytes.
- Consider validating enum scans defensively:
  - `JobStatus.Scan` / `RunStatus.Scan` accept any string (`internal/store/models.go:26`, `internal/store/models.go:114`); DB enums help, but code won’t catch drift if schema and code diverge.

## Streamlining / Simplification

- Consolidate migrations story; remove dead paths.
  - Either remove `schema_version` machinery + tests (`internal/store/versioning.go:9`, `internal/store/migrate_test.go:9`), or make `RunMigrations` create/populate `schema_version`.
  - Keep one implementation path; delete unused helpers (`internal/store/sql_split.go:17`) if not used.
- Make query ordering deterministic where ties can happen.
  - Add tie-breakers (e.g. `ORDER BY step_index ASC, id ASC`) for claims/lists (`internal/store/queries/jobs.sql:126`).
- Avoid `SELECT *` for blob-heavy list endpoints.
  - Logs/diffs/artifacts/events list queries use `SELECT *` (e.g. `internal/store/queries/logs.sql:6`, `internal/store/queries/diffs.sql:9`, `internal/store/queries/artifact_bundles.sql:6`, `internal/store/queries/events.sql:6`).
  - Consider “metadata list” queries and separate “get blob by id” queries to reduce I/O.

## Suggested Minimal Slices

- Slice 1: Set connection `search_path` in `internal/store/store.go` (or enforce DSN format everywhere).
- Slice 2: Make migrations + tests consistent (choose: “schema.sql only” vs “schema_version tracked”).
