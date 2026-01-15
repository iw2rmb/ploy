# Roadmap: `--follow` (repo job graph follow)

## Goal

Change the meaning of `--follow` for:
- `ploy mod run` (both the spec-based single-repo form and the mod-project form),
- `ploy run` (single-repo submission),
- `ploy pull`,

so that `--follow` **does not stream stdout/stderr logs** from mod containers, and instead renders a **summarized per-repo job graph** until completion.

This roadmap is for future work. It documents intended behavior at HEAD and what will change.

## Current state (HEAD)

### Existing `--follow` behavior (logs/events)

`ploy mod run --follow` currently streams run events and log events:
- Flag parsing and docs: `cmd/ploy/mod_run_flags.go` (`--follow`, `--cap`, `--cancel-on-cap`, `--max-retries`, `--log-format`)
- Follow implementation: `cmd/ploy/mod_run_exec.go` (`followRunEvents`)
- SSE client and event decoding:
  - `internal/cli/mods/events.go` consumes `event: run`, `event: stage`, `event: log`, `event: retention` from `GET /v1/runs/{id}/logs`
  - `internal/stream/hub.go` documents emitted event types (`log`, `retention`, `run`, `stage`, `done`)

`ploy run` (single-repo submission via flags in `cmd/ploy/run_submit.go`) does not support `--follow` today.

`ploy mod run <mod-id|name>` (mod-project run in `cmd/ploy/mod_run_project.go`) does not support `--follow` today.

### Existing “logs” command

Log streaming remains available via:
- `ploy run logs <run-id>` (`cmd/ploy/run_commands.go` → `internal/cli/mods/logs.go`)

This roadmap keeps `ploy run logs` as the log streaming surface.

## New `--follow` meaning

When `--follow` is set, the CLI prints a summarized job graph per repo and refreshes it until the run reaches a terminal state.

Required output shape (conceptual):
```
<repo>
  <step index> <job type> <job id> <mod name> <spinner> <duration> <status>
  ...
```

### Definitions

Repo header (`<repo>`):
- Prefer `repo_url` for display.
- For runs with missing `repo_url` (should be rare; `GET /v1/runs/{id}/repos` attempts to populate it), fall back to `repo_id`.

Row fields:
- `<step index>`: numeric ordering value (matches jobs.step_index / `domaintypes.StepIndex`).
- `<job type>`: the phase kind (matches jobs.mod_type: `pre_gate`, `mod`, `post_gate`, `heal`, `re_gate`, `mr`).
- `<job id>`: jobs.id (KSUID).
- `<mod name>`:
  - for `job type=mod`: display the associated spec step name if available; otherwise fall back to job name (e.g. `mod-0`).
  - for non-`mod` phases: use a fixed label (e.g. `pre-gate`, `post-gate`, `heal`, `re-gate`) or the job name.
- `<spinner>`:
  - `Running`: animated spinner glyph
  - non-running: a stable glyph (or blank) to avoid flicker
- `<duration>`:
  - `Running`: `now - started_at`
  - terminal: `duration_ms` (or `finished_at - started_at` if duration is not present)
- `<status>`: job status (Created/Queued/Running/Success/Fail/Cancelled) normalized for display (e.g. lowercase).

### Non-goals

- `--follow` will not print per-container stdout/stderr logs.
- No compatibility flag to restore the old `--follow` behavior.
  - Use `ploy run logs ...` for logs.

## Data requirements (and gaps at HEAD)

To render the required rows, the CLI needs (per repo, per job):
- job id, name, mod_type, step_index, status
- started/finished timestamps or duration
- a “mod name” for `mod` steps

At HEAD:
- `GET /v1/runs/{id}/repos` exists and provides repo-level metadata (`internal/server/handlers/runs_batch_http.go` → `RunRepoResponse` in `internal/server/handlers/runs_batch_types.go`).
- There is no control-plane API to list jobs for a specific repo execution.
- `GET /v1/runs/{id}/status` returns a run-wide stages map keyed by job id (`internal/server/handlers/mods_ticket.go`), but it does not include:
  - job name,
  - job mod_type,
  - job duration,
  - repo attribution for a job.

## Proposed API additions

### New endpoint: list repo jobs

Add:
- `GET /v1/runs/{run_id}/repos/{repo_id}/jobs`

Semantics:
- Returns jobs for the **current attempt** of the repo execution (default), with an optional `?attempt=N` override.
- Jobs ordered by `(step_index ASC, name ASC, id ASC)` to ensure stable UI ordering.

Response shape (proposal):
```json
{
  "run_id": "…",
  "repo_id": "…",
  "attempt": 1,
  "jobs": [
    {
      "job_id": "…",
      "name": "mod-0",
      "mod_type": "mod",
      "step_index": 2000,
      "status": "Running",
      "started_at": "…",
      "finished_at": null,
      "duration_ms": 1234,
      "display_name": "java17-upgrade"
    }
  ]
}
```

Implementation hooks:
- Store access already exists for server-side filtering: `st.ListJobsByRunRepoAttempt` is used in `internal/server/handlers/events_repo.go` and `internal/server/handlers/runs_batch_http.go`.

### “mod name” / display name source

To support `<mod name>`:
- Extend job creation to persist a display name for `mod` steps.

Proposed storage:
- Add a field to `jobs.meta` (JSONB) when creating jobs from spec in `internal/server/handlers/mods_ticket.go`:
  - For `mod-i` jobs: `meta.mods_step_name = ModsSpec.Steps[i].Name` (if non-empty).
  - If step name is empty: omit `meta.mods_step_name` and let clients fall back to job name.

Rationale:
- The control plane does not expose a GET endpoint to fetch spec bodies at HEAD (`docs/api/OpenAPI.yaml` shows only `POST /v1/mods/{mod_id}/specs`), so the CLI cannot reliably derive step names for mod-project runs without a new spec read API.

## CLI implementation (follow engine)

### Rendering model

Follow loop:
1. Resolve run id (command-specific; see below).
2. Resolve repos in the run via `GET /v1/runs/{id}/repos` (existing).
3. For each repo, fetch jobs via `GET /v1/runs/{run_id}/repos/{repo_id}/jobs` (new).
4. Render the per-repo graphs to stderr, refreshing on changes until terminal.

Refresh trigger:
- Prefer using the existing SSE stream `GET /v1/runs/{id}/logs` as the “change signal”:
  - subscribe via `internal/cli/stream.Client` (already used by `internal/cli/mods/events.go`),
  - ignore `event: log` payloads for output purposes,
  - on `event: run` / `event: stage`, refresh graphs by re-fetching job lists.

This keeps `--max-retries` semantics (SSE reconnection) meaningful without requiring a new SSE contract.

### Command integration points

Spec-based single-repo Mods run:
- Current: `cmd/ploy/mod_run_exec.go` uses `followRunEvents` and `internal/cli/mods.EventsCommand`.
- Change: wire `--follow` to the new follow engine and stop printing log lines there.

Mod-project run (`ploy mod run <mod>`):
- Current: `cmd/ploy/mod_run_project.go` submits the run and prints run_id only.
- Change: add `--follow` support and call the follow engine when set.

Single-repo run submit (`ploy run --repo ...`):
- Current: `cmd/ploy/run_submit.go` submits and prints `run_id`.
- Change: add `--follow` support and call the follow engine when set.

`ploy pull`:
- Current: does not exist at HEAD (see `roadmap/pull.md`).
- Change: `--follow` follows the initiated/reused run and then proceeds to pull diffs once succeeded.

## What remains unchanged

- The log streaming surface is `ploy run logs ...` (`cmd/ploy/run_commands.go` + `internal/cli/mods/logs.go`).
- SSE event types and payloads for `/v1/runs/{id}/logs` remain as-is (see `docs/mods-lifecycle.md` § “SSE Contract” and `internal/stream/hub.go`).

## Docs to update after implementation

- `docs/mods-lifecycle.md`:
  - update the “CLI Surfaces for Mods” section where it currently says `--follow` streams logs/events.
  - ensure the SSE contract description remains accurate (it can still describe `event: log`, but `--follow` will no longer render it).
- `cmd/ploy/README.md`: update all examples and text describing `--follow`.
- `tests/e2e/mods/README.md`: update the “Streaming Events and Reconnection” section to reflect graph-follow output (and keep SSE reconnection notes only where relevant).
- `docs/api/OpenAPI.yaml`:
  - add the new `GET /v1/runs/{run_id}/repos/{repo_id}/jobs` path and schemas.
