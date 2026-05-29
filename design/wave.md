# Wave-Based Run Model

## Summary

Replace the current batch-run model with a wave-based model:

- A `mig` is a long-lived migration project with a spec and managed repo set.
- A `wave` is one launch of work for a mig or single-repo run.
- A `run` is exactly one repository execution.

The target shape is `mig -> waves -> runs -> jobs`. A multi-repo mig launch creates
one wave and one run per selected repo. A single-repo `ploy run` creates a one-run
wave. The run itself becomes the repo execution boundary, so user-facing run
operations no longer need a second `repo_id` selector.

## Scope

In scope:

- PostgreSQL schema and sqlc queries for `waves`, `runs`, and removal of
  `run_repos` as an execution table.
- Server handlers for single-repo run submission, mig wave creation, wave status,
  wave cancellation, run status, run restart, run artifacts, run diffs, run jobs,
  run snapshot, job logs, and pull resolution.
- Scheduler, job claim, completion, stale recovery, and run-status reconciliation.
- Node claim payload, snapshot hydration, sticky workspace paths, repo-local
  artifact paths, and node-side upload paths.
- CLI status, follow, patch/apply, artifact fetch, mig run follow, mig pull, and
  diagnostics output that currently resolves a repo inside a run.
- OpenAPI, lifecycle docs, and tests covering the public contract.

Out of scope:

- Backward-compatible aliases for repo-scoped run endpoints.
- Legacy-shape rejection guards or tests that enumerate old `run_repos` contract
  states.
- Keeping a run that contains multiple repositories.
- Changing the mig spec schema or job-chain semantics except where the run/repo
  identity boundary requires it.
- Changing global repo identity storage in `repos` or mig membership storage in
  `mig_repos`.

No backward compatibility is required. Removed endpoints, fields, and CLI flags
should disappear from the current contract rather than being preserved with
compatibility shims.

## Why This Is Needed

The current model makes `runs` look like the user-facing execution object, but the
actual repo execution lives in `run_repos`. A single-repo `ploy run` is documented
and implemented as a degenerate batch: one `runs` row plus one `run_repos` row.
This leaks into almost every surface:

- Commands like `run apply` must resolve a repo inside a run even when there is
  only one repo.
- APIs are mostly shaped as `/v1/runs/{run_id}/repos/{repo_id}/...`.
- Jobs, snapshots, artifacts, diffs, logs, and recovery are scoped by
  `(run_id, repo_id, attempt)`.
- Run completion is derived from all `run_repos` rows instead of the run's own job
  chain.
- Multi-repo mig launches need one operator handle, but `run_id` currently serves
  both as that handle and as an individual execution identifier.

The wave model separates those concepts. `wave_id` is the operator handle for a
launch. `run_id` is the execution handle for one repo. That removes the nested
repo selector from run operations while preserving a first-class grouping object
for multi-repo mig work.

## Goals

- Make `runs` the only execution state for one repository.
- Make `waves` the only grouping state for one launch.
- Ensure every run has exactly one `wave_id` and one `repo_id`.
- Ensure every wave has one or more runs.
- Keep single-repo `ploy run` simple: one wave, one run, no visible repo selector.
- Keep multi-repo mig operations ergonomic: one wave ID for follow, status,
  cancel, and artifact collection.
- Move repo snapshot fields, SHA seed fields, attempt, status, timing, and error
  state from `run_repos` onto `runs`.
- Keep jobs as the execution units and preserve current linked job-chain behavior.
- Make API and CLI contracts describe current state only, with no old-shape
  compatibility layer.

## Non-Goals

- No support for a run containing more than one repo.
- No nullable `wave_id` path for single-repo runs.
- No compatibility routes under `/v1/runs/{run_id}/repos/{repo_id}/...`.
- No compatibility fields that mirror old `run_repos` names.
- No new project-level repo model; `repos` and `mig_repos` remain the repo identity
  and mig-membership tables.
- No change to stored repository URLs beyond the existing credential-free,
  normalized URL rule.

## Current Baseline (Observed)

- `internal/store/schema.sql` defines `runs` as a run-level row with `mig_id`,
  `spec_id`, and coarse status only. It also defines `run_repos` as the per-repo
  execution table with `(run_id, repo_id)` primary key, refs, `source_commit_sha`,
  `repo_sha0`, `status`, `attempt`, `last_error`, and timing fields.
- `internal/store/schema.sql` defines `jobs` with `run_id`, `repo_id`,
  `repo_base_ref`, and `attempt`, so job scoping is currently
  `(run_id, repo_id, attempt)`.
- `internal/store/queries/run_repos.sql` owns creation, lookup, status updates,
  attempt increments, failed-repo selection, repo-history listing, snapshot
  metadata, and queued-work discovery for run repos.
- `internal/store/queries/jobs.sql` claims and promotes jobs by joining jobs to
  `run_repos` and filtering by `run_id`, `repo_id`, and `attempt`.
- `internal/server/handlers/runs_submit.go` implements `POST /v1/runs` as a
  single-repo submit, but it creates a mig, spec, mig repo, run, and one
  `run_repos` row.
- `internal/server/handlers/migs_runs.go` implements
  `POST /v1/migs/{mig_id}/runs` by selecting many mig repos, creating one run,
  and creating many `run_repos` rows.
- `internal/server/handlers/runs_batch_scheduler.go` starts queued work by
  listing `run_repos` for a run and creating or promoting jobs for each
  `(run_id, repo_id, attempt)`.
- `internal/server/recovery/reconcile.go` updates `run_repos.status` from job
  outcomes and marks `runs.status` finished only when all `run_repos` rows are
  terminal.
- `internal/server/handlers/register.go` exposes repo-scoped run APIs:
  `/v1/runs/{run_id}/repos`, `/snapshot`, `/restart`, `/diffs`, `/logs`,
  `/artifacts`, `/jobs`, and `/cancel`.
- `internal/server/handlers/runs_repo_snapshot.go` materializes a snapshot by
  loading `GetRunRepoSnapshotMetadata(run_id, repo_id)` and authorizing the node
  with `HasRunningJobForRunRepoNode`.
- `internal/server/handlers/diffs.go`, `artifacts_repo.go`, `events.go`, and
  `runs_repo_jobs.go` all require `(run_id, repo_id)` and then list jobs for the
  current repo attempt.
- `internal/server/handlers/pull.go` resolves `POST /v1/runs/{run_id}/pull` by
  matching a repo URL against all repos in a run and resolves
  `POST /v1/migs/{mig_id}/pull` by selecting the latest terminal `run_repos` row.
- `internal/cli/runs/report_builder.go` builds a run report by calling
  `/v1/runs/{run_id}/repos`, then calling repo-scoped jobs and diffs endpoints for
  each repo.
- `ploy run apply` currently resolves a repo inside a run before downloading
  the accumulated patch for that repo.
- `internal/nodeagent/execution_paths_test.go` documents node-side durable
  artifact paths under `runs/<run-id>/repos/<repo-id>/...`.
- `docs/migs-lifecycle.md` documents the current model as batched runs using one
  `runs` row with per-repo `run_repos` rows, and describes single-repo runs as
  degenerate batches.

## Target Contract

### Identity Model

`wave_id` is a public, stable ID for one launch. It is used to group runs created
by the same submit operation.

`run_id` is a public, stable ID for one repository execution. A run never contains
more than one repo.

`repo_id` continues to identify a row in `repos`. It is stored on `runs` for
attribution and snapshot materialization, but it is not required in user-facing run
routes.

Required invariants:

- Every run has exactly one non-null `wave_id`.
- Every run has exactly one non-null `repo_id`.
- Every run has immutable `spec_id`, `repo_base_ref`, `source_commit_sha`,
  `source_commit_sha`, and `repo_sha0` values.
- Every run has one current `attempt`.
- Every wave has one or more runs.
- A wave belongs to one mig and one spec snapshot.
- A run belongs to the same mig and spec snapshot as its wave.
- Multi-repo launch parallelism is represented by many runs in one wave, not by
  many repos inside one run.

### Schema Contract

Add `waves`:

```sql
CREATE TABLE waves (
  id           TEXT PRIMARY KEY,
  mig_id       TEXT NOT NULL REFERENCES migs(id) ON DELETE RESTRICT,
  spec_id      TEXT NOT NULL REFERENCES specs(id) ON DELETE RESTRICT,
  created_by   TEXT,
  status       run_status NOT NULL DEFAULT 'Started',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  stats        JSONB NOT NULL DEFAULT '{}'::jsonb
);
```

Update `runs` to own repo execution:

```sql
CREATE TABLE runs (
  id                 TEXT PRIMARY KEY,
  wave_id            TEXT NOT NULL REFERENCES waves(id) ON DELETE CASCADE,
  mig_id             TEXT NOT NULL REFERENCES migs(id) ON DELETE RESTRICT,
  spec_id            TEXT NOT NULL REFERENCES specs(id) ON DELETE RESTRICT,
  repo_id            TEXT NOT NULL REFERENCES repos(id) ON DELETE RESTRICT,
  repo_base_ref      TEXT NOT NULL,
  source_commit_sha  TEXT NOT NULL,
  source_commit_sha  TEXT NOT NULL DEFAULT '',
  repo_sha0          TEXT NOT NULL DEFAULT '',
  created_by         TEXT,
  status             run_repo_status NOT NULL DEFAULT 'Queued',
  attempt            INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  last_error         TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at         TIMESTAMPTZ,
  finished_at        TIMESTAMPTZ,
  stats              JSONB NOT NULL DEFAULT '{}'::jsonb,
  CONSTRAINT runs_mig_repo_membership_fkey
    FOREIGN KEY (mig_id, repo_id)
    REFERENCES mig_repos(mig_id, repo_id)
    ON DELETE RESTRICT
);
```

Indexes:

- `waves_mig_created_idx` on `(mig_id, created_at DESC, id DESC)`.
- `waves_status_idx` on active statuses.
- `runs_wave_created_idx` on `(wave_id, created_at ASC, id ASC)`.
- `runs_status_idx` on active statuses.
- `runs_repo_created_idx` on `(repo_id, created_at DESC, id DESC)`.
- `runs_mig_repo_created_idx` on `(mig_id, repo_id, created_at DESC, id DESC)`.

Drop `run_repos` after all query and code paths have moved. There is no
compatibility view.

### Status Contract

Use existing status concepts, but move their ownership:

- `waves.status`: `Started | Finished | Cancelled`.
- `runs.status`: `Queued | Running | Success | Fail | Cancelled`.
- `jobs.status`: unchanged.

Wave derived status is computed from child runs:

- `pending`: no child run is running and at least one child run is queued.
- `running`: at least one child run is running.
- `success`: all child runs are `Success`.
- `failed`: at least one child run is `Fail` and none are running.
- `cancelled`: at least one child run is `Cancelled` and none are running.

Completion rules:

- A run reaches a terminal status when all jobs for its current attempt are
  terminal.
- A wave reaches `Finished` when all child runs are terminal and none are
  cancelled by wave cancellation.
- A wave reaches `Cancelled` when wave cancellation is requested.
- A single-run wave follows the same rules as a multi-run wave.

### HTTP API Contract

Single-repo run submission:

```http
POST /v1/runs
```

Request remains the single-repo submit shape:

```json
{
  "repo_url": "https://gitlab.example/org/repo.git",
  "base_ref": "master",
  "source_commit_sha": "0123456789abcdef0123456789abcdef01234567",
  "spec": {},
  "created_by": "optional"
}
```

Response:

```json
{
  "wave_id": "wave-id",
  "run_id": "run-id",
  "mig_id": "mig-id",
  "spec_id": "spec-id"
}
```

The handler creates a one-run wave. It resolves `source_commit_sha` before
creating durable rows.

Multi-repo mig launch:

```http
POST /v1/migs/{mig_id}/waves
```

Request:

```json
{
  "repo_selector": {
    "mode": "all"
  },
  "created_by": "optional"
}
```

`repo_selector.mode` remains `all | failed | explicit`. Explicit mode keeps the
current `repos` array of repo URLs.

Response:

```json
{
  "wave_id": "wave-id",
  "mig_id": "mig-id",
  "spec_id": "spec-id",
  "run_count": 10
}
```

Wave status:

```http
GET /v1/waves/{wave_id}
GET /v1/waves/{wave_id}/runs
POST /v1/waves/{wave_id}/cancel
```

`GET /v1/waves/{wave_id}` returns wave identity, mig identity, spec identity,
created metadata, child run counts by status, and derived status.

`GET /v1/waves/{wave_id}/runs` returns the member run summaries with repo URL,
refs, source SHA, status, attempt, timing, and last error.

`POST /v1/waves/{wave_id}/cancel` cancels all active child runs and active jobs
for those runs.

Run APIs:

```http
GET  /v1/runs/{run_id}
GET  /v1/runs/{run_id}/status
POST /v1/runs/{run_id}/cancel
POST /v1/runs/{run_id}/restart
GET  /v1/runs/{run_id}/snapshot
GET  /v1/runs/{run_id}/diffs
GET  /v1/runs/{run_id}/artifacts
GET  /v1/runs/{run_id}/jobs
POST /v1/runs/{run_id}/pull
```

Removed run APIs:

```http
POST /v1/migs/{mig_id}/runs
GET  /v1/runs/{run_id}/repos
POST /v1/runs/{run_id}/repos
GET  /v1/runs/{run_id}/repos/{repo_id}/snapshot
POST /v1/runs/{run_id}/repos/{repo_id}/restart
GET  /v1/runs/{run_id}/repos/{repo_id}/diffs
GET  /v1/runs/{run_id}/repos/{repo_id}/logs
GET  /v1/runs/{run_id}/repos/{repo_id}/artifacts
GET  /v1/runs/{run_id}/repos/{repo_id}/jobs
POST /v1/runs/{run_id}/repos/{repo_id}/cancel
```

Run pull resolution:

- `POST /v1/runs/{run_id}/pull` no longer needs `repo_url` to choose a repo.
- It returns the run's `repo_id` for clients that still need
  branch naming metadata.
- `POST /v1/migs/{mig_id}/pull` selects the latest terminal run for the requested
  repo URL and status mode.
- Add `POST /v1/waves/{wave_id}/pull` only if wave-scoped artifact or patch
  collection needs a direct wave endpoint during implementation. The initial
  contract does not require it because `GET /v1/waves/{wave_id}/runs` plus
  run-scoped APIs is sufficient.

### CLI Contract

`ploy run` commands operate on one run:

- `ploy run status <run-id>` shows one repo execution.
- `ploy run apply <run-id>` does not accept repo selector flags.
- `ploy run pull <run-id>` downloads artifacts for one run.
- `ploy run cancel <run-id>` cancels one run.

`ploy mig run` creates a wave:

- It prints `wave_id`.
- In JSON mode it includes `wave_id`, `mig_id`, `spec_id`, and `run_count`.
- Follow mode follows the wave, not a parent run.

`ploy mig status` and `ploy mig pull` operate across waves/runs:

- Mig status lists waves and their aggregate counts.
- Mig pull resolves the latest successful or failed run for the current repo.

Wave commands may be added under `ploy wave` if direct operator access is useful:

```bash
ploy wave status <wave-id> {--json | --follow}
ploy wave cancel <wave-id>
ploy wave runs <wave-id>
```

If added, these commands are thin clients over `/v1/waves/{wave_id}` and
`/v1/waves/{wave_id}/runs`.

### Node Runtime Contract

Node claim responses include `run_id`, `wave_id`, and `repo_id`. `repo_id` remains
available for labels, diagnostics, and repo attribution, but execution scoping is
the run.

Snapshot hydration changes from:

```text
GET /v1/runs/{run_id}/repos/{repo_id}/snapshot
```

to:

```text
GET /v1/runs/{run_id}/snapshot
```

Authorization checks that the requesting node owns a running job for `run_id`.

Node-side durable paths change from:

```text
runs/<run-id>/repos/<repo-id>/...
```

to:

```text
runs/<run-id>/...
```

Repo ID may remain in log fields and artifact metadata, but it must not be needed
to find the execution directory.

## Implementation Notes

### Store and Schema

- Add `waves` schema, generated queries, and domain/API types.
- Move repo execution columns from `run_repos` to `runs`.
- Change `runs.status` to use the per-run execution status enum. Keep
  `run_status` for waves or rename enums only if the migration stays simple and
  mechanical.
- Replace `CreateRunWithRepos` with `CreateWaveWithRuns`.
- Replace run repo queries with run queries:
  - create run execution;
  - get run execution;
  - list runs by wave;
  - list active runs by wave;
  - count runs by status for wave;
  - increment run attempt;
  - update run refs;
  - update run error;
  - list failed repo IDs by mig from latest terminal runs;
  - get latest run by mig, repo, and terminal status;
  - get run snapshot metadata.
- Update job queries to scope by `(run_id, attempt)` instead of
  `(run_id, repo_id, attempt)`.
- Keep `jobs.repo_id` only if it remains useful for attribution and indexes. It
  must mirror `runs.repo_id`; execution logic must not require it as a selector.

### Server Handlers

- Rename mig batch creation from `createMigRunHandler` to wave creation and mount
  it at `POST /v1/migs/{mig_id}/waves`.
- Change `createSingleRepoRunHandler` to create a one-run wave.
- Add wave handlers for get, list runs, and cancel.
- Replace repo-scoped run handlers with run-scoped handlers.
- Change snapshot handler to load metadata from `runs`.
- Change diff/artifact/log/jobs handlers to list jobs by run attempt directly.
- Change pull resolution so a run has one repo by definition.
- Update route registration and OpenAPI verification in one pass so public docs
  and runtime routes stay aligned.

### Scheduler and Recovery

- Replace `BatchRepoStarter` with a starter that finds queued runs, creates the job
  chain for each run, and promotes the next job for each run attempt.
- Change job claim eligibility to join jobs to `runs`, not `run_repos`.
- Change completion reconciliation:
  - derive one run's terminal status from its jobs;
  - update `runs.status` and `runs.last_error`;
  - complete the wave when all child runs are terminal.
- Change stale-job recovery to group by `(run_id, attempt)`.
- Change cancellation:
  - run cancel cancels active jobs for one run;
  - wave cancel cancels active runs and active jobs for all runs in the wave.

### Nodeagent

- Update claim payload types and manifest mutation to include `wave_id` where useful.
- Update snapshot download URLs and authorization headers.
- Update sticky workspace and repo-local artifact paths to be run-scoped.
- Update upload clients for diffs, logs, artifacts, and job completion only where
  route shapes or payloads change.
- Preserve the current repo SHA chain behavior: first job uses `runs.repo_sha0`,
  later jobs use predecessor output SHA.

### CLI

- Update `internal/cli/runs/report_builder.go` so one run report fetches one run's
  jobs, diffs, artifacts, and logs directly.
- Remove repo-resolution branches from run apply and artifact commands.
- Update mig run command to call `POST /v1/migs/{mig_id}/waves` and follow a wave.
- Add wave status/follow rendering if the implementation exposes `ploy wave`.
- Update help goldens and completion artifacts after Cobra command changes.

### Docs and OpenAPI

- Replace the `docs/migs-lifecycle.md` batch-run section with the wave model after
  implementation.
- Update all OpenAPI paths and schemas for wave and run-scoped endpoints.
- Remove `RunRepo` and `RunRepoCounts` public schemas when they are no longer
  referenced.
- Keep design history in this file only; long-lived docs should describe the
  shipped wave model after implementation.

## Milestones

### 1. Schema and Store Model

Scope:

- Add `waves`.
- Move execution fields onto `runs`.
- Replace run repo queries with run and wave queries.
- Regenerate sqlc.

Expected result:

- Store can create one wave with one or more one-repo runs atomically.
- Store can list and count runs by wave.
- Store no longer needs `run_repos` for new execution paths.

Testable outcome:

- Store tests cover wave creation, one-run wave creation, failed-repo selection,
  latest terminal run lookup, and cancellation primitives.

### 2. Server Execution Path

Scope:

- Update submit, mig launch, scheduler, claim, completion, recovery, and cancel
  paths.
- Add wave handlers.
- Replace repo-scoped run handlers with run-scoped handlers.

Expected result:

- Single-repo and multi-repo launches create waves and one-repo runs.
- Jobs execute and complete without `run_repos`.
- Wave status reflects child run state.

Testable outcome:

- Handler and recovery tests prove single-run and multi-run waves complete,
  fail, and cancel correctly.

### 3. Node Runtime Paths

Scope:

- Update claim payload, snapshot route, hydration, sticky workspace paths, and
  artifact paths.

Expected result:

- A node can execute a claimed run using only `run_id` as the execution directory
  key.
- Snapshot download uses `/v1/runs/{run_id}/snapshot`.

Testable outcome:

- Nodeagent tests verify path layout, snapshot URL shape, and SHA verification.

### 4. CLI and Operator Surface

Scope:

- Update run status/follow/report, run apply, run pull/fetch, mig run, mig
  pull, and optional wave commands.

Expected result:

- Run commands no longer ask for repo selection.
- Mig launch returns and follows a wave.
- Mig pull resolves the latest terminal run for the current repo.

Testable outcome:

- CLI tests cover JSON output, human output, follow behavior, patch/apply without
  repo flags, and artifact fetch.

### 5. Docs, OpenAPI, and Cleanup

Scope:

- Remove old repo-scoped route docs.
- Update lifecycle docs.
- Update OpenAPI schemas and route verification.
- Delete obsolete run repo tests and fixtures.

Expected result:

- Public docs and generated API surface describe only the wave model.
- No `run_repos` execution references remain outside deleted migration history.

Testable outcome:

- `go test ./docs/api ./internal/cli/... ./internal/server/... ./internal/store/...`
  passes.
- `go test ./...` passes.
- Documentation link checks pass if docs under `docs/**` are changed.

## Acceptance Criteria

- `run_repos` is removed as an execution table.
- Every run has a non-null `wave_id`.
- Every run has exactly one `repo_id`.
- A multi-repo mig launch creates one wave and N runs.
- A single-repo run creates one wave and one run.
- No public run route contains `/repos/{repo_id}`.
- Run status, jobs, diffs, logs, artifacts, restart, cancel, and snapshot all work
  with only `run_id`.
- Wave status and cancellation work with only `wave_id`.
- Job scheduling, claiming, completion, and stale recovery use `(run_id, attempt)`.
- Node artifact and workspace paths are run-scoped, not run-plus-repo-scoped.
- `ploy run` commands do not expose repo selection for an existing run.
- `ploy mig run` returns `wave_id` and can follow aggregate wave progress.
- OpenAPI and `docs/migs-lifecycle.md` describe the new model.
- Tests fail if a future change reintroduces a multi-repo run contract.

## Risks

- The schema change has a large blast radius because `run_repos` is currently the
  center of scheduling, status, and artifact lookup.
- Some code uses `runs.status` as a coarse parent status and some uses
  `run_repos.status` as execution status; merging them onto `runs` requires
  careful type and naming cleanup.
- Existing artifact bundles, diffs, and logs are stored by `run_id` and `job_id`,
  so most blobs survive the model change, but list and path code may still assume
  a repo selector.
- Node-side paths and API-visible artifact paths must change together to avoid
  failed-job bundles being uploaded to paths the server no longer lists.
- Wave follow output can become noisy for large waves unless it supports compact
  aggregate rendering plus drill-down to individual runs.
- Partial launch failure during source SHA resolution must remain atomic: either
  no wave is created, or the created wave contains only durable runs whose source
  commit was resolved.

## References

- `docs/runs.md` for the current `ploy run` command contract that benefits from
  one-repo runs.
- `docs/migs-lifecycle.md` for the current documented batch model that must be
  replaced after implementation.
- `internal/store/schema.sql` for current `runs`, `run_repos`, `jobs`, artifacts,
  diffs, and logs schema.
- `internal/store/queries/run_repos.sql` and `internal/store/queries/jobs.sql` for
  current execution-scoped queries.
- `internal/server/handlers/runs_submit.go` for current single-repo submit.
- `internal/server/handlers/migs_runs.go` for current multi-repo batch creation.
- `internal/server/handlers/runs_batch_scheduler.go` for current queued repo
  scheduling.
- `internal/server/recovery/reconcile.go` for current completion derivation.
- `internal/server/handlers/register.go` for current repo-scoped public routes.
- `internal/server/handlers/runs_repo_snapshot.go` for current repo-scoped
  snapshot materialization.
- `internal/cli/runs/report_builder.go` and `internal/cli/run/run_apply.go` for
  current CLI repo resolution inside a run.
