# Migs Lifecycle

The current execution model is:

```text
mig -> waves -> runs -> jobs
```

- A `mig` is a long-lived project with a current spec and managed repo set.
- A `wave` is one launch of work for a mig.
- A `run` is one repository execution in that wave.
- A `job` is one claimable execution unit in a run.

Single-repo `ploy run` creates a private mig, one wave, and one run. Multi-repo
`ploy mig run` creates one wave and one run per selected repository.

## State

Wave status:

```text
Started -> Finished | Cancelled
```

Run status:

```text
Queued -> Running -> Success | Fail | Cancelled
```

Job status:

```text
Created -> Queued -> Running -> Success | Fail | Error | Cancelled
```

The scheduler finds waves with queued runs, creates the run job chain when
needed, and marks the run `Running` before jobs are claimed. A run reaches a
terminal state when its current attempt's jobs are terminal. A wave reaches
`Finished` when all child runs are terminal, unless it is explicitly cancelled.

## Schema

Durable execution state lives in:

- `migs`: project identity and current spec pointer.
- `mig_repos`: managed repo membership and mutable source ref.
- `waves`: one launch grouping row.
- `runs`: one repo execution row, including `wave_id`, `repo_id`,
  `repo_base_ref`, `source_commit_sha`, `repo_sha0`, `attempt`, `status`, and
  `last_error`.
- `jobs`: job chain rows scoped operationally by `(run_id, attempt)`; `repo_id`
  remains on each row for attribution.

`run_id` is sufficient for run operations. The repo selector is stored on the
run and is not part of public run routes.

## APIs

Create work:

- `POST /v1/runs`
- `POST /v1/migs/{mig_id}/waves`

Wave operations:

- `GET /v1/waves/{wave_id}`
- `GET /v1/waves/{wave_id}/runs`
- `POST /v1/waves/{wave_id}/cancel`

Run operations:

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

Job operations stay job-scoped:

- `GET /v1/jobs/{job_id}/status`
- `GET /v1/jobs/{job_id}/logs`
- `POST /v1/jobs/{job_id}/complete`

## Runtime Paths

Node-local run state is stored under:

```text
$PLOYD_CACHE_HOME/runs/{run_id}/workspace
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/in
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/out
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/stdout.log
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/stderr.log
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/diff.patch
```

The node downloads source snapshots from `GET /v1/runs/{run_id}/snapshot`.

## Production Migration

`ployd` applies embedded Tern migrations on startup and stores migration state in
`ploy.tern_schema_version`. Fresh databases run the full migration chain.
Existing databases at the final custom migration version are baselined to Tern
version `1`, then cleanup migration `2` runs normally. Older pre-current
databases are not upgraded automatically.
