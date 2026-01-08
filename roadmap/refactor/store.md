# Store Refactor Notes (`internal/store`)

## Workspace State

- `git diff` was empty at time of review; `git status` had untracked `roadmap/refactor/*`.
- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs, StepIndex).

## High-Risk Bugs / Correctness

- Missing `search_path` on app connections.
  - Schema writes everything under `ploy` (`internal/store/schema.sql:7`), but sqlc queries use unqualified table names (e.g. `internal/store/queries/jobs.sql:2`).
  - `internal/store/store.go:28` does not set `RuntimeParams["search_path"]`; correctness depends on DSN including it.
  - Solution: set `RuntimeParams["search_path"] = "ploy,public"` in `internal/store/store.go` when constructing the pool config, so correctness does not depend on external DSN formatting.
- `RunMigrations` is inconsistent and likely broken for multi-statement schema execution.
  - It claims “supports multi-statement files” but uses `pool.Exec(ctx, schemaSQL)` (`internal/store/migrations.go:48`).
  - `ensureVersionTable` explicitly avoids multi-statement issues (`internal/store/versioning.go:11`), and `execMigrationSQL` exists but is unused (`internal/store/sql_split.go:17`).
  - Migration tests assume `ploy.schema_version` exists and has entries (`internal/store/migrate_test.go:30`), but `internal/store/schema.sql` has no `schema_version` table.
  - Solution (Option A, tracked): add `ploy.schema_version` to `internal/store/schema.sql`, make `RunMigrations` use `execMigrationSQL`, and keep/update `internal/store/versioning.go` + migration tests.
- Job claiming and scheduling are under-specified and race-prone.
  - `ClaimJob` orders globally by `step_index` only (`internal/store/queries/jobs.sql:126`), despite `step_index` being a per-run/per-repo sequencing mechanism (`internal/store/schema.sql:164`).
  - `ClaimJob` uses `FOR UPDATE SKIP LOCKED` with a join to `runs` (`internal/store/queries/jobs.sql:119`); it may lock `runs` rows unnecessarily.
  - `ClaimJob(ctx, nodeID *string)` permits `nil`, which can produce `status='Running'` with `node_id=NULL` (`internal/store/querier.go:25`, `internal/store/queries/jobs.sql:131`).
  - `ScheduleNextJob` selects without `FOR UPDATE SKIP LOCKED` and updates without re-checking status (`internal/store/queries/jobs.sql:175`).
  - `duration_ms` can become NULL if `started_at` is NULL (`internal/store/queries/jobs.sql:218`), violating `jobs.duration_ms BIGINT NOT NULL` (`internal/store/schema.sql:196`).
  - Solution:
    - Define the claim/schedule ordering explicitly in SQL (add tie-breakers; at minimum `ORDER BY step_index ASC, id ASC`), and scope ordering to the correct domain (per-run/per-repo vs global).
    - Make `ClaimJob` require a non-null node id at the API boundary (see “Type-System Hardening”) and enforce it in SQL (no `node_id=NULL` writes).
    - Make scheduling atomic (select row `FOR UPDATE SKIP LOCKED`, update with a status predicate) so multiple schedulers cannot race.
    - Compute `duration_ms` defensively (treat missing `started_at` as 0 or set `started_at` on transition into `Running`).

## Type-System Hardening

- Use sqlc overrides (`sqlc.yaml:1`) to map DB identifiers to domain newtypes instead of `string`:
  - IDs: `runs.id`, `jobs.id`, `mods.id`, `mod_repos.id`, `nodes.id`, `specs.id` are plain `string` (`internal/store/models.go:149`).
  - Solution: follow `roadmap/refactor/contracts.md` § "IDs and Newtypes (`internal/domain/types`)" (sqlc overrides + end-to-end typed IDs).
- Make `step_index` harder to misuse:
  - DB type is `FLOAT` (`internal/store/schema.sql:191`) and Go type is `float64` (`internal/store/models.go:226`).
  - If floats are kept, wrap in a dedicated type and validate “integer-like” invariants at boundaries.
  - Solution: follow `roadmap/refactor/contracts.md` § "StepIndex (Ordering Invariant)" (map `jobs.step_index` to `types.StepIndex`, validate via `StepIndex.Valid()`).
- Avoid nullable “must-be-present” inputs at the store boundary:
  - `ClaimJob(ctx, nodeID *string)` (`internal/store/querier.go:25`) should be non-nullable (wrapper method or query change).
  - Solution: change the store interface to `ClaimJob(ctx, nodeID types.NodeID)` (or add a wrapper `ClaimJobForNode`) and remove the `*string` escape hatch.
- Reduce `[]byte` ambiguity for JSONB columns:
  - `Job.Meta`, `Run.Stats`, `Spec.Spec`, `Diff.Summary` are `[]byte` (`internal/store/models.go:201`, `internal/store/models.go:304`, `internal/store/models.go:330`).
  - Prefer `json.RawMessage` or typed structs at API/service boundaries to prevent storing non-JSON bytes.
  - Solution: keep sqlc raw bytes if desired, but require JSON validation before insert/update at the boundary (server/workflow), and prefer `json.RawMessage` in those boundary structs.
- Consider validating enum scans defensively:
  - `JobStatus.Scan` / `RunStatus.Scan` accept any string (`internal/store/models.go:26`, `internal/store/models.go:114`); DB enums help, but code won’t catch drift if schema and code diverge.
  - Solution: add explicit allow-lists in `Scan` (return error on unknown) so drift fails fast in tests and at startup.

## Streamlining / Simplification

- Make query ordering deterministic where ties can happen.
  - Solution: apply tie-breakers consistently to all list queries that order by a non-unique column (and follow `roadmap/refactor/contracts.md` § "StepIndex (Ordering Invariant)" for anything ordered by `step_index`).
- Avoid `SELECT *` for blob-heavy list endpoints.
  - Logs/diffs/artifacts/events list queries use `SELECT *` (e.g. `internal/store/queries/logs.sql:6`, `internal/store/queries/diffs.sql:9`, `internal/store/queries/artifact_bundles.sql:6`, `internal/store/queries/events.sql:6`).
  - Consider “metadata list” queries and separate “get blob by id” queries to reduce I/O.
  - Solution: add `List*Meta` queries returning only ids + timestamps + small fields; keep existing `Get*` queries for blob fetches.

## Suggested Minimal Slices

- Slice 1: Set connection `search_path` in `internal/store/store.go` (or enforce DSN format everywhere).
- Slice 2: Implement tracked migrations (add `ploy.schema_version`, use `execMigrationSQL`, keep/update versioning + tests).
