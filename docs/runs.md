# Runs And Waves

`ploy run` submits one repository execution. A run is the execution boundary:
jobs, logs, diffs, artifacts, snapshots, apply, pull, status, and cancellation
are addressed by `run_id`.

A `wave` groups one launch. Single-repo submit creates one wave with one run.
Mig launches create one wave with one run per selected repo.

## Submit

```bash
ploy run <spec-path>|<named-spec> [<repo-path>|<namespace/repo[:ref]>] [--apply] [--pull[=path]] [--build-gate-forced <lang>@<release>[/<tool>]] [--gitlab-token-env ENV_NAME|--gitlab-token-prompt]
ploy mig run <mig-id|name> [<namespace/repo[:ref]> ... | --failed] [--follow] [--json] [--gitlab-token-env ENV_NAME|--gitlab-token-prompt]
```

- `ploy run` prints `run_id` and `mig_id`.
- `--gitlab-token-env ENV_NAME` reads a run-scoped ephemeral GitLab token from
  the named environment variable. `--gitlab-token-prompt` prompts for the token.
  The flags are mutually exclusive. The server stores only a SHA-256 marker in
  run metadata and keeps the token in memory while the run or wave is active.
  Ephemeral GitLab tokens require a configured GitLab domain and are accepted
  only for HTTPS repos on that host.
- `POST /v1/runs` returns `wave_id`, `run_id`, `mig_id`, and `spec_id`.
- Named-spec submissions pass `spec_selector`. The server refreshes the
  configured Git repositories, compiles the selected YAML, and stores an
  immutable spec snapshot with its repository URL, YAML path, and full source
  SHA. Local file and directory submissions remain client-compiled.
- `ploy run ... --env:<step> KEY=VALUE` overrides `steps[].envs` for exactly
  one step named `<step>`. The flag is repeatable, values may be empty, and
  later values win for the same step/key. For named specs, the CLI sends the
  ordered overrides to the server. The server applies them after spec
  composition and before it expands the remaining environment placeholders.
  An override therefore replaces the selected source value before that source
  value can cause an unresolved-placeholder error.
- Named-spec compilation can read only the server process variables named by
  `PLOY_NAMED_SPECS_ENVS_ALLOWLIST`. The server stores expanded values in the
  immutable canonical spec snapshot. See [Environment variables](envs/README.md#server-control-plane)
  for configuration and security constraints.
- `ploy run ... --build-gate-forced <lang>@<release>[/<tool>]` overrides both
  `build_gate.pre.stack` and `build_gate.post.stack` with `mode: forced`.
  Use `--build-gate-forced-pre` or `--build-gate-forced-post` to override one
  phase. The global flag cannot be combined with phase-specific flags. These
  overrides create missing `build_gate`, `pre`, or `post` objects, preserve
  `build_gate.images`, and set `build_gate.disabled: false`. For named specs,
  the server applies the structured phase overrides after compilation.
- `ploy mig run` prints `wave_id`; `--json` prints `wave_id`, `mig_id`,
  `spec_id`, and `run_count`.
- Remote selector expansion is server-owned through `POST /v1/repos/resolve`.
- `ploy mig run` resolves positional repo selectors to repo identity for
  explicit selection; the run source ref still comes from the mig repo's stored
  `base_ref`.
- Mig wave creation uses `POST /v1/migs/{mig_id}/waves`.

## Inspect And Control

```bash
ploy run ls [--all] [--limit N] [--offset N]
ploy run status <run-id> [--json|--follow]
ploy run sbom {pre|post|diff} <run-id>
ploy run cancel <run-id>
ploy run restart <run-id> [--from-failed] [--gitlab-token-env ENV_NAME|--gitlab-token-prompt]
ploy wave status <wave-id> [--follow]
ploy wave runs <wave-id>
ploy wave cancel <wave-id>
ploy job status <job-id>
ploy job log <job-id>
```

Text and terminal status output can include browser links. These links never
include bearer tokens or other URL credentials. The browser must have its own
authenticated session.

`ploy run ls` shows `ID STATUS SPEC REPO`. Without `--all`, the server filters
to the authenticated token username, falling back to the CLI user's `$USER`
when the token has no username. Named specs render as
`domain/namespace/repo:spec-name`; anonymous specs render as `spec_id`.

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

Job-scoped API surfaces:

- `GET /v1/jobs/{job_id}/status`
- `GET /v1/jobs/{job_id}/logs`
- `POST /v1/jobs/{job_id}/report`

Jobs can POST raw UTF-8 text to the report endpoint using worker authentication
and the `PLOY_NODE_UUID` header. Each POST replaces `jobs.meta.report`; an empty
body clears it. Reports are limited to 1 MiB and survive job completion.
`ploy run status` shows a `[R]EPORT` preview for running and completed jobs.
In follow mode, press `r` to expand or collapse the full report. Status JSON
contains the original report text. Terminal rendering removes control characters
other than newlines and tabs.

Every job receives its authoritative `PLOY_JOB_ID`. Migration containers also
receive the node-owned server URL, node identity, and a read-only worker bearer
header file at `PLOY_WORKER_AUTH_HEADER_FILE`. The node stages this private file
outside uploaded artifacts and removes it after execution. Configured worker
TLS files are also mounted read-only. These credentials use
the existing worker trust boundary; they are not job-scoped credentials.

Run inspection, artifacts, diffs, jobs, logs, cancellation, restart, and pull
resolution are all addressed by `run_id`; `repo_id` is returned only as
attribution metadata.

`ploy run restart` locks the terminal run, increments the attempt, clears
previous run stats, and creates the new job chain in one transaction. The
returned run is `Running` and eligible for node execution.
When a new ephemeral GitLab token is provided, the restarted attempt receives a
new server-generated SHA-256 marker. Ephemeral GitLab tokens require a
configured GitLab domain and are accepted only when every target repo uses
HTTPS and its host matches that configured domain.

`ploy run restart <run-id> --from-failed` retries the failed step in the
current attempt. It preserves successful jobs and queues only the failed job.
Its cancelled, unexecuted successors return to `Created` and advance normally.
The server rejects active runs, cancelled waves, inconsistent job chains, and
another active run for the repository. A run with status `Cancelled` is eligible only when an actual failed step
caused cancellation of its successors. An explicitly cancelled chain is rejected.

This mode requires the retained workspace on the original node. It preserves
partial edits left by the failed step; it does not restore the step's original
input. If the workspace is missing, the node fails the retry without downloading
the original source snapshot. Use a full restart in that case. Both the server
and node must support `from_failed` before using this mode.

A retry keeps job IDs and increments `resume_count` in run stats. It clears the
retried jobs' current logs, metrics, SBOM rows, and execution results. Existing
uploaded artifact bundles remain available. New log streams start a new
execution; nodes echo the resume count in log uploads and completion statistics.
The server rejects requests from an earlier execution.

`ploy run sbom pre|post|diff <run-id>` reads persisted package rows from the
current run attempt. The `diff` view omits unchanged package versions and marks
changed, added, and removed package versions.

## Artifacts And Apply

```bash
ploy run pull <run-id> [artifacts-path]
ploy run apply <run-id> [path] [--force]
```

`run pull` downloads final artifacts into a directory. By default, `run apply`
applies the accumulated run patch only when the local git worktree is clean,
the local origin matches the run `repo_url`, and local `HEAD` matches the run
`source_commit_sha`. Repository matching ignores the SSH or HTTPS transport,
SSH user and port, and the trailing `.git` suffix. `--force` skips all three
safety checks. It does not suppress run lookup, patch download, or `git apply`
errors.

Nodes upload final repo artifacts when a job fails or errors, when `post_gate`
succeeds, and when a terminal `mig` succeeds for a run with
`build_gate.disabled: true`.

## Storage

Node-local run state uses these paths:

```text
$PLOYD_CACHE_HOME/runs/{run_id}/workspace
$PLOYD_CACHE_HOME/runs/{run_id}/share
$PLOYD_CACHE_HOME/runs/{run_id}/runtime-share
$PLOYD_CACHE_HOME/runs/{run_id}/jobs/{job_id}/{cache,home,in,out,staging,tmp}
$PLOYD_CACHE_HOME/runs/{run_id}/jobs/{job_id}/{stdout.log,stderr.log,diff.patch,container.inspect.json}
```

The node removes `cache`, `home`, `staging`, and `tmp` after job execution.
The node retains the other job paths as durable artifacts. Artifact bundles map
the durable paths to `artifacts/{job_id}/...` and map `share` to
`artifacts/shared/...`. The bundle format does not expose the host layout.
The persisted `container.inspect.json` omits the container arguments,
environment, command, and entrypoint because these fields can contain
credentials.

The control plane stores launch grouping in `waves`, execution state in `runs`,
and work units in `jobs`.
