# Runs And Waves

`ploy run` submits one repository execution. A run is the execution boundary:
jobs, logs, diffs, artifacts, snapshots, apply, pull, status, and cancellation
are addressed by `run_id`.

A `wave` groups one launch. Single-repo submit creates one wave with one run.
Mig launches create one wave with one run per selected repo.

## Submit

```bash
ploy run <spec-path> [<repo-path>|<namespace/repo[:ref]>] [--apply] [--pull[=path]]
ploy mig run <mig-id|name> [<namespace/repo[:ref]> ... | --failed] [--follow] [--json]
```

- `ploy run` prints `run_id` and `mig_id`.
- `POST /v1/runs` returns `wave_id`, `run_id`, `mig_id`, and `spec_id`.
- `ploy mig run` prints `wave_id`; `--json` prints `wave_id`, `mig_id`,
  `spec_id`, and `run_count`.
- Remote selector expansion is server-owned through `POST /v1/repos/resolve`.
- `ploy mig run` resolves positional repo selectors to repo identity for
  explicit selection; the run source ref still comes from the mig repo's stored
  `base_ref`.
- Mig wave creation uses `POST /v1/migs/{mig_id}/waves`.

## Inspect And Control

```bash
ploy run status <run-id> [--json|--follow]
ploy run sbom {pre|post|diff} <run-id>
ploy run cancel <run-id>
ploy run restart <run-id>
ploy wave status <wave-id> [--follow]
ploy wave runs <wave-id>
ploy wave cancel <wave-id>
ploy job log <job-id>
```

Run-scoped API surfaces:

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
- `GET /v1/runs/{run_id}/sbom/{pre|post|diff}`

Wave-scoped API surfaces:

- `POST /v1/migs/{mig_id}/waves`
- `GET /v1/waves/{wave_id}`
- `GET /v1/waves/{wave_id}/runs`
- `POST /v1/waves/{wave_id}/cancel`

Run inspection, artifacts, diffs, jobs, logs, cancellation, restart, and pull
resolution are all addressed by `run_id`; `repo_id` is returned only as
attribution metadata.

`ploy run sbom pre|post|diff <run-id>` reads persisted package rows from the
current run attempt. The `diff` view omits unchanged package versions and marks
changed, added, and removed package versions.

## Artifacts And Apply

```bash
ploy run pull <run-id> [artifacts-path]
ploy run apply <run-id> [path] [--force]
```

`run pull` downloads final artifacts into a directory. `run apply` applies the
accumulated run patch into a clean local git worktree. The local origin must
match the run `repo_url`. Local `HEAD` must match the run `source_commit_sha`;
`--force` bypasses only that source-commit guard.

## Storage

Node-local durable state is rooted at:

```text
$PLOYD_CACHE_HOME/runs/{run_id}/workspace
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts
$PLOYD_CACHE_HOME/runs/{run_id}/artifacts/{job_id}/{in,out,stdout.log,stderr.log,diff.patch}
```

The control plane stores launch grouping in `waves`, execution state in `runs`,
and work units in `jobs`.
