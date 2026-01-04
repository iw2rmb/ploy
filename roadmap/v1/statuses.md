# Roadmap v1 — Status Model

## Goal

Define a new status model that:

- Removes `assigned` (run-level node assignment is obsolete when each job can be claimed by different nodes).
- Keeps run status high-level and repo-aggregate.
- Makes repo status the authoritative per-repo execution state.

This document is a design note for what must change in code to implement the new model.

## Status enums (v1)

### `runs.status` (new `run_status`)

Canonical values:

- `Started` (replaces HEAD `running` / “active run”)
- `Cancelled` (new terminal; set by `POST /v1/runs/{run_id}/cancel`)
- `Finished` (new terminal; set when all repos are terminal)

Notes:

- There is no `Queued` run status in v1. A run exists in `Started` even if it has repos that are still `Queued`.
- `assigned` must be removed entirely (DB enum + store + query assumptions).

### `run_repos.status` (new `run_repo_status`)

Canonical values:

- `Queued` (renamed from HEAD `pending`)
- `Running` (same meaning as HEAD `running`)
- `Cancelled` (harmonize spelling across all status enums; HEAD mixes `canceled` and `cancelled`)
- `Fail` (renamed from HEAD `failed`)
- `Success` (renamed from HEAD `succeeded`)

Notes:

- v1 removes `skipped` from `run_repo_status`.
  - HEAD uses `skipped` only for repo removal (`DELETE /v1/runs/{run_id}/repos/{repo_id}` in `internal/server/handlers/runs_batch_http.go`).
  - v1 removes that endpoint, so there is no “repo removal → skipped” behavior to preserve.

## Transition rules (v1)

### Job queueing rules (v1)

Claiming stays global, but “what becomes claimable next” is repo-scoped.

- Claimable jobs have `jobs.status='Queued'`.
- Non-claimable jobs have `jobs.status='Created'`.
- On job creation for a repo attempt:
  - the first job (lowest `step_index`) is inserted as `Queued`
  - all later jobs are inserted as `Created`
- On job success:
  - find the next job for the same repo attempt by `(run_id, repo_id, attempt) ORDER BY step_index`
  - promote it `Created → Queued`
- On job failure:
  - do not promote the next “normal” step
  - if healing inserts a job, that healing job is inserted as `Queued` for the current `(run_id, repo_id, attempt)`

### Run status transitions

- Initial: `Started` (on run creation).
- `Started` → `Cancelled`:
  - triggered by `POST /v1/runs/{run_id}/cancel`.
  - endpoint behavior: see `roadmap/v1/api.md` (`POST /v1/runs/{run_id}/cancel`).
- `Started` → `Finished`:
  - checked after a job completes **when it is the last job for that repo**.
  - set to `Finished` only when all repos are terminal:
    - terminal repo statuses: `Cancelled`, `Fail`, `Success`

Timestamps: see `roadmap/v1/db.md`.

MR note (align with HEAD semantics):

- MR jobs (`jobs.mod_type='mr'`) are **auxiliary post-run jobs** and must not affect `run_repos.status` or `runs.status`.
  - HEAD reference: `maybeScheduleMRJobForRun` comment in `internal/server/handlers/nodes_complete_mr.go`.
- “Run is Finished” must therefore ignore MR jobs in any terminal-state aggregation.

### Run repo status transitions

- Initial: `Queued`.
- `Queued` → `Running` when a job for that repo is claimed (i.e. when some `jobs.status` transitions to `Running` for that `(run_id, repo_id, attempt)`), and the repo is not already `Running`.
- `Running` → terminal:
  - `Success` if the repo’s job sequence completes successfully.
  - `Fail` if the repo’s job sequence fails (and is not cancelled).
  - `Cancelled` if the repo is cancelled, or the parent run is cancelled.

Timestamps: see `roadmap/v1/db.md`.

## Required code changes (by area)

### Additional status-dependent codepaths (HEAD)

These files currently branch on the HEAD status enums and will need updates (or removal) as part of the v1 status model change:

- Run submission / job creation:
  - `internal/server/handlers/mods_ticket.go` (creates runs with `RunStatusQueued`; creates jobs with `JobStatusQueued/Created`)
- Cancel/resume:
  - `internal/server/handlers/mods_cancel.go` (sets `runs.status=canceled`; cancels jobs)
  - `internal/server/handlers/mods_resume.go` (resets jobs; sets `runs.status=queued`; branches on `queued/assigned/running/succeeded/failed/canceled`)
- Completion paths that set or interpret job/run terminal states:
  - `internal/server/handlers/nodes_complete_healing.go` (inserts healing jobs; branches on terminal job statuses)
  - `internal/server/handlers/nodes_complete_mr.go` (MR creation logic currently branches on `runs.status` terminal values)

### Database schema

Update `internal/store/schema.sql`:

- Replace `run_status` enum:
  - remove: `queued`, `assigned`, `running`, `succeeded`, `failed`, `canceled`
  - add: `Started`, `Cancelled`, `Finished`
- Replace `run_repo_status` enum:
  - remove: `pending`, `skipped`, `cancelled`, `succeeded`, `failed`
  - add: `Queued`, `Running`, `Cancelled`, `Fail`, `Success`
- Remove (or deprecate) `runs.node_id`:
  - HEAD still has `runs.node_id` and `RunStatusAssigned`; both reflect “run assigned to a node”.
  - v1 uses job-level `jobs.node_id` only.

Impact:

- `internal/store/models.go` is sqlc-generated and will change after schema changes.

### Jobs status enum

v1 also changes job status strings.

Update `internal/store/schema.sql` `job_status` enum:

- Capitalize all values (e.g. `Created`, `Queued`, `Running`, `Success`, `Fail`, `Cancelled`).
- Rename `canceled` → `Cancelled`.
- Rename `pending` → `Queued`.
- Remove `skipped` from `job_status` entirely.
  - HEAD note: `skipped` is treated as terminal in a few places, but there is no request path that sets it today (job completion only accepts `succeeded|failed|canceled`).

Supporting code to remove/update when `skipped` is removed:

- `internal/store/models.go` + `internal/store/status_conversion.go` (+ tests) — drop `JobStatusSkipped` and `"skipped"` conversion/validation.
- `internal/server/handlers/nodes_complete_run.go` — remove `JobStatusSkipped` from terminal aggregation.
- `internal/server/handlers/jobs_complete.go` — remove `JobStatusSkipped` from “schedule next job” logic.
- `internal/mods/api/status_conversion.go` — remove `JobStatusSkipped` mapping (status no longer exists).
- `internal/workflow/graph/types.go` (+ any builders) — remove `NodeStatusSkipped`.

### Store status conversion helpers

Update these to match v1 canonical values:

- `internal/store/status_conversion.go`
- `internal/store/status_conversion_test.go`

### Store SQL queries that depend on run status strings

Update these to stop referencing `queued/assigned/running/succeeded/failed/canceled` and to use v1 values:

- `internal/store/queries/runs.sql`
  - `AckRunStart` currently transitions `queued/assigned` → `running`.
  - v1 does not need this transition at run level; runs are created `Started`.
- `internal/store/queries/run_repos.sql`
  - multiple queries hardcode `pending/running/succeeded/failed/skipped/cancelled` and set `finished_at` based on those strings.
  - v1 must update these to `Queued/Running/Cancelled/Fail/Success` and ensure timestamps follow the new model.
- `internal/store/queries/jobs.sql`
  - `ClaimJob` currently allows claims only when `runs.status IN ('queued','running')` (and special-cases MR jobs).
  - v1 should allow claims only when `runs.status='Started'` for normal jobs.
  - v1 must keep the MR special-case aligned with HEAD:
    - MR jobs are claimable after the run is terminal (v1: when `runs.status='Finished'`), and MR failures do not change `runs.status`/`run_repos.status`.
    - HEAD reference: `internal/store/queries/jobs.sql` `mod_type='mr'` branch + `internal/server/handlers/nodes_complete_mr.go`.

### Server handlers

Run status updates / endpoint naming:

- `internal/server/handlers/register.go`
  - rename `POST /v1/runs/{run_id}/stop` → `POST /v1/runs/{run_id}/cancel` (v1 API).
- `internal/server/handlers/runs_batch_http.go`
  - `stopRunHandler` currently sets `runs.status=canceled` and marks only `run_repos.status='pending'` repos as `cancelled` (it does not cancel running repos).
  - v1 cancel must:
    - set `runs.status=Cancelled`
    - cancel all repos (`Queued`/`Running` → `Cancelled`)
    - cancel/remove waiting jobs from the queue
- `internal/server/handlers/nodes_claim.go`
  - today a successful claim may call `AckRunStart` to transition `runs.status` to `running`.
  - v1 must remove that transition (run is created `Started`) and ensure claim logic does not depend on queued/assigned.
  - v1 must set `run_repos.status=Running` for the claimed job’s `(run_id, repo_id, attempt)` (idempotent); timestamps are defined in `roadmap/v1/db.md`.
- `internal/server/handlers/runs_batch_scheduler.go`
  - today it finds runs with `run_repos.status='pending'` repos via `ListBatchRunsWithPendingRepos` (which filters by `runs.status IN ('queued','assigned','running')`).
  - v1 must update scheduling to the new run status model (`Started/Cancelled/Finished`).

Run completion:

- `internal/server/handlers/jobs_complete.go`
  - currently:
    - schedules next job with `ScheduleNextJob(run_id)` (run-scoped),
    - then calls `maybeCompleteMultiStepRun` to set run terminal status (`succeeded/failed/canceled`).
  - v1 must change the terminal computation:
    - compute and persist the repo terminal status (`run_repos.status`) when a repo’s last job completes
    - compute `runs.status` (`Finished`) by aggregating repo statuses
    - remove any run-level `succeeded/failed` transitions; success/failure becomes repo-scoped.
- `internal/server/handlers/nodes_complete_run.go`
  - `maybeCompleteMultiStepRun` currently derives run terminal state from jobs and writes `runs.status=succeeded/failed/canceled`.
  - v1 must be rewritten (or replaced) to update:
    - `run_repos.status` for the repo whose job completed
    - `runs.status=Finished` only when all repos are terminal

Batch status helpers:

- `internal/server/handlers/runs_batch_types.go`
  - update `isTerminalRunStatus` and `deriveBatchStatus` to the new statuses.
  - remove `DerivedStatusCompleted` or rename if the external derived status vocabulary changes.

### CLI + OpenAPI surfaces

Not implemented in this doc, but required for consistency:

- Server endpoint rename: `POST /v1/runs/{run_id}/cancel` (not `/stop`).
- OpenAPI path file currently documents `/v1/runs/{run_id}/stop` at HEAD (`docs/api/paths/runs_id_stop.yaml`).
- `internal/mods/api/status_conversion.go`
  - converts between store status enums and the external Mods API types (`RunState`, `StageState`).
  - v1 status renames/casing must be reflected in these conversions.
- `internal/nodeagent/statusuploader.go`
  - uploads job terminal status strings to the server.
  - v1 must ensure the node sends the v1 job status strings (`Success`/`Fail`/`Cancelled`, etc.), not HEAD lower-case values.

## Status spelling and casing (must be consistent)

v1 must keep these consistent across DB, store types, and API responses:

- `internal/store/schema.sql`
- sqlc-generated `internal/store/models.go`
- `internal/store/status_conversion.go`
- handler comparisons and API responses
