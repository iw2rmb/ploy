# Wave-Based Run Model

## Summary

The shipped execution model is:

```text
mig -> waves -> runs -> jobs
```

- A `mig` is a long-lived migration project with a current spec and managed repo
  set.
- A `wave` is one launch of work.
- A `run` is exactly one repository execution.
- A `job` is one claimable execution unit in a run.

Single-repo `ploy run` creates a private mig, one wave, and one run. Multi-repo
`ploy mig run` creates one wave and one run per selected repo.

No backward compatibility aliases are part of the current contract.

## Current Contract

`wave_id` is the public launch handle. It is used for wave status, wave run
listing, and wave cancellation.

`run_id` is the public execution handle. It is used for status, follow, logs,
jobs, diffs, artifacts, snapshot hydration, pull resolution, restart, and
cancellation.

`repo_id` remains stored on the run and jobs for attribution and repo-centric
history. It is not a selector in public run routes.

Required invariants:

- Every run has one non-null `wave_id`.
- Every run has one non-null `repo_id`.
- Every run stores immutable `mig_id`, `spec_id`, `repo_base_ref`,
  `source_commit_sha`, and `repo_sha0` values.
- Every wave has one `mig_id`, one `spec_id`, and one or more runs.
- Multi-repo launch parallelism is represented by many runs in one wave.

## API

Create work:

- `POST /v1/runs`
- `POST /v1/migs/{mig_id}/waves`

`POST /v1/runs` returns:

```json
{
  "wave_id": "2...",
  "run_id": "2...",
  "mig_id": "abc12345",
  "spec_id": "def67890"
}
```

`POST /v1/migs/{mig_id}/waves` returns:

```json
{
  "wave_id": "2...",
  "mig_id": "abc12345",
  "spec_id": "def67890",
  "run_count": 3
}
```

Wave routes:

- `GET /v1/waves/{wave_id}`
- `GET /v1/waves/{wave_id}/runs`
- `POST /v1/waves/{wave_id}/cancel`

Run routes:

- `GET /v1/runs/{run_id}`
- `GET /v1/runs/{run_id}/status`
- `POST /v1/runs/{run_id}/cancel`
- `POST /v1/runs/{run_id}/restart`
- `POST /v1/runs/{run_id}/pull`
- `GET /v1/runs/{run_id}/jobs`
- `GET /v1/runs/{run_id}/diffs`
- `GET /v1/runs/{run_id}/logs`
- `GET /v1/runs/{run_id}/artifacts`
- `GET /v1/runs/{run_id}/snapshot`

## CLI

`ploy run` submits one repository execution and keeps its human output focused on
the created run and mig IDs.

`ploy mig run` prints the created `wave_id` in human mode. With `--json`, it
prints the full wave creation response: `wave_id`, `mig_id`, `spec_id`, and
`run_count`.

`ploy mig run` accepts optional positional `namespace/repo[:ref]` selectors. The
CLI resolves them to repo URLs for explicit repo identity selection; launch refs
are still snapshotted from the mig repo set.

`ploy run ls` lists runs. Empty output is `No runs found.`

## Scheduler

The wave scheduler periodically lists waves with queued runs. For each wave, it
creates or advances job chains for queued runs and marks those runs `Running`
before nodes claim jobs.

The scheduler uses wave and run terminology in package names, symbols, logs, and
errors:

- `internal/store/wavescheduler`
- `WaveRunStarter`
- `StartQueuedRuns`
- `StartQueuedRunsResult`

The config key is `scheduler.wave_scheduler_interval`; the matching environment
override is `PLOYD_SCHEDULER_WAVE_SCHEDULER_INTERVAL`.

## Storage

Durable execution state lives in:

- `migs`: project identity and current spec pointer.
- `mig_repos`: managed repo membership and mutable source ref.
- `waves`: launch grouping rows.
- `runs`: repository execution rows.
- `jobs`: job chain rows scoped operationally by `(run_id, attempt)`.

Node-local runtime state is rooted by run:

```text
$PLOYD_CACHE_HOME/runs/{run_id}/workspace
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/{in,out,stdout.log,stderr.log,diff.patch}
```

## Migration

The one-time production schema rewrite is embedded in `ployd` startup. It only
runs when the old pre-wave schema shape is detected. The manual SQL wrapper is
kept in `scripts/migrate_wave_model_20260601.sql` for controlled operations.

After that rewrite, normal execution paths use only `waves`, `runs`, and `jobs`.
