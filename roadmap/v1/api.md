# Roadmap v1 — API

## Mod projects

Change entry: repurpose `/v1/mods` from “run submission” to “mod project CRUD”.

- Current (HEAD): `POST /v1/mods` submits a run (see `internal/server/handlers/register.go`, `submitRunHandler` in `internal/server/handlers/mods_ticket.go`, and `docs/api/OpenAPI.yaml`).
- Proposed (v1): `/v1/mods` becomes mod project CRUD (create/list/delete/archive), and mod runs are created under `/v1/mods/{mod_id}/runs`.
- Where: new handlers under `internal/server/handlers/*` + OpenAPI updates in `docs/api/OpenAPI.yaml`.
- Compatibility: breaking API change; no backward compatibility required.
- Unchanged: `/v1/runs/*` remains the run execution/history surface (cancel/status/SSE), updated to support repo scoping for multi-repo runs.
  - Endpoint rename: `POST /v1/runs/{id}/stop` becomes `POST /v1/runs/{id}/cancel` (see current `stopRunHandler` in `internal/server/handlers/runs_batch_http.go`).

### `POST /v1/mods`

Creates a mod project.

Request:

- `name` (string, unique)
- optional `spec` (object; JSON) — creates an initial spec row and sets `mods.spec_id`

Response:

- `id`, `name`, `created_at`, optional `spec_id` (when `spec` is provided)

Behavior:

- If `spec` is provided, set `mods.spec_id` to the created `spec_id`.

### `GET /v1/mods`

Lists mods.

Query:

- `limit`, `offset`
- optional filters:
  - `name_substring` (substring match on `mods.name`)
  - `archived` (`true` → only archived, `false` → only active)

### `DELETE /v1/mods/{mod_id}`

Deletes a mod.

Behavior:

- Refuse deletion if any runs exist for the mod (`runs.mod_id == mod_id`).

## Pulling diffs for a mod repo

### `POST /v1/mods/{mod_id}/pull`

Selects the latest run for a specific repo in a mod and returns the repo-scoped execution identifiers needed to pull diffs.

This is the API behind `ploy mod pull`.

Request:

- `repo_url`
- optional `mode`:
  - `last-failed`: newest terminal `Fail`
  - `last-succeeded` (default): newest terminal `Success`

Response:

- `run_id`
- `repo_id` (`mod_repos.id`, aka `mod_repo_id`)
- `repo_target_ref` (`run_repos.repo_target_ref`)

Notes:

- Server performs the lookup using `mod_id + repo_url` → `mod_repos.id`, then selects the appropriate `run_repos` by `run_repos.created_at DESC` (joining through `runs` by `runs.id` and filtering by `runs.mod_id`) and filtering by the requested terminal status.
- Diffs are then listed via `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` and downloaded via `GET /v1/diffs/{diff_id}?download=true`.

### `PATCH /v1/mods/{mod_id}/archive`

Archives a mod.

- When a mod is archived, it cannot be executed (any attempt to create a mod run must fail).
- A mod cannot be archived while it has any running jobs.

### `PATCH /v1/mods/{mod_id}/unarchive`

Unarchives a mod.

## Single-repo runs

Change entry: add `POST /v1/runs` for single-repo submission.

- Current (HEAD): there is no run-submission endpoint at `POST /v1/runs`; run submission uses `POST /v1/mods` (see `internal/server/handlers/register.go`).
- Proposed (v1): `POST /v1/runs` submits a single-repo run and starts execution; it may create a mod project as a side-effect.
- Where: new handler under `internal/server/handlers/*`, CLI callers under `cmd/ploy/*` and `internal/cli/*`, OpenAPI updates in `docs/api/OpenAPI.yaml`.
- Compatibility: breaking for clients that submit runs via `POST /v1/mods`; no backward compatibility required.
- Unchanged: existing batch lifecycle endpoints under `/v1/runs/*` remain (cancel/status/SSE).
  - Endpoint rename: `POST /v1/runs/{id}/stop` becomes `POST /v1/runs/{id}/cancel`.

### `POST /v1/runs`

Submits a single-repo run and immediately starts execution.

This is the API behind `ploy run --spec ... --repo ...`.

Side-effects:

- Creates a mod project; the created mod has `name == id`.
- Creates an initial spec row for that mod from the provided `spec` and sets `mods.spec_id`.
- Creates a mod repo row for the provided `repo_url` (identity within the mod).

Request:

- `repo_url`
- `base_ref`
- `target_ref`
- `spec` (object; JSON)

Response:

- `run_id`
- `mod_id`
- `spec_id`

## Specs

Change entry: specs are stored globally; mods point at a single `spec_id`.

- Current (HEAD): no spec storage outside `runs.spec` JSONB.
- Proposed (v1): specs are stored in `specs` (global dictionary); `mods.spec_id` points at the current spec.
- Where: `internal/store/schema.sql` (`specs`, `mods.spec_id`) + mod CRUD handlers.
- Compatibility: breaking; no backward compatibility required.
- Unchanged: runs are immutable and reference the exact `spec_id` that was current on the mod at run creation time.

### `POST /v1/mods/{mod_id}/specs`

Creates a new `specs` row and updates `mods.spec_id` to point at it.

Request:

- optional `name` (string)
- `spec` (object; JSON)

Response:

- `id` (`spec_id`), `created_at`

## Repo set

### `POST /v1/mods/{mod_id}/repos`

Adds/enables a repo in a mod.

Request:

- `repo_url`
- `base_ref`
- `target_ref`

Response:

- `id` (mod_repo_id), plus stored fields.

### `POST /v1/mods/{mod_id}/repos/bulk`

Bulk upsert repos for a mod from CSV.

Request:

- `Content-Type: text/csv`
- Body is UTF-8 CSV with header row: `repo_url,base_ref,target_ref`

Behavior:

- Continues on per-line errors; may partially apply.
- Upserts by `(mod_id, repo_url)`:
  - inserts new `mod_repos` rows
  - updates `base_ref` / `target_ref` for existing rows
- Does not affect historical run data (`run_repos.repo_base_ref` / `run_repos.repo_target_ref` snapshots remain unchanged).

Response:

- counts: `created`, `updated`, `failed`
- `errors`: array of `{line, message}`

### `GET /v1/mods/{mod_id}/repos`

Lists repos for a mod.

### `DELETE /v1/mods/{mod_id}/repos/{mod_repo_id}`

Deletes a repo from the mod repo set.

- Refuse deletion if the repo has historical executions (referenced by `run_repos.repo_id`).

## Running a mod

### `POST /v1/mods/{mod_id}/runs`

Creates a batch run from the mod + spec + selected repos and immediately starts it.

This is the API behind `ploy mod run <mod> ...`.

Request:

- `repo_selector`:
  - `{ "mode": "all" }`
  - `{ "mode": "failed" }` (repos whose last terminal state is `Fail`)
  - `{ "mode": "explicit", "repos": ["<repo_url>", ...] }`
- optional `created_by`

Behavior:

- Use `mods.spec_id`; if `mods.spec_id` is NULL, return an error that spec is required.
- v1: no per-run ref overrides; `run_repos.repo_base_ref` / `run_repos.repo_target_ref` are copied from `mod_repos` at run creation time.

Response:

- `run_id`
- `started`, `already_done`, `queued`

## Schema/docs updates

- Add OpenAPI schemas for `Mod`, `Spec`, `ModRepo`, `CreateModRunRequest`, `CreateModRunResponse`.
- Move run-scoped “artifacts” endpoints that currently live under `/v1/mods/{run_id}/*` to a run-scoped namespace to avoid colliding with `/v1/mods` (mod projects).
- For multi-repo runs, repo-scoped artifacts are addressed under `/v1/runs/{run_id}/repos/{repo_id}/...` where `repo_id` is `mod_repos.id` (aka `mod_repo_id`).
- Repo-scoped endpoints required for CLI workflows (must exist under the repo-scoped namespace):
  - `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` — list diffs for this repo execution in this run.
  - `GET /v1/runs/{run_id}/repos/{repo_id}/logs` — SSE logs/events stream for this repo execution.
  - `GET /v1/runs/{run_id}/repos/{repo_id}/artifacts` — list artifacts produced by jobs for this repo execution.
  - `POST /v1/runs/{run_id}/repos/{repo_id}/cancel` — cancel this repo execution (v1 replacement for HEAD `DELETE /v1/runs/{id}/repos/{repo_id}`).
  - `DELETE /v1/runs/{run_id}/repos/{repo_id}` — delete the `run_repos` record (only allowed when there were no jobs; otherwise refuse).
  - `POST /v1/runs/{run_id}/repos/{repo_id}/restart` — restart a repo execution (HEAD reference: `restartRunRepoHandler` in `internal/server/handlers/runs_batch_http.go`).
- Keep existing `/v1/runs/*` APIs as the run execution/history surface; mod APIs are just project/spec/repo management + run creation.

## Node job claiming (unchanged, v1)

v1 keeps the existing node claim flow:

- `POST /v1/nodes/{id}/claim` remains a **global** “next job” claim endpoint (no repo selector).
- Repo-scoped execution ordering is enforced by server-side progression rules (see `roadmap/v1/db.md` “Repo-scoped scheduling invariant (v1)”).

## v0 → v1 endpoint mapping notes (codebase reference)

These are concrete v0 routes that currently exist in the server (see `internal/server/handlers/register.go`) and how v1 needs to reinterpret them.

- v0 run submission:
  - v0: `POST /v1/mods` submits a single-repo run.
  - v1: `POST /v1/mods` creates a mod project; run submission is `POST /v1/runs`.
- v0 run “artifacts” surface under `/v1/mods/{run_id}/*`:
  - v0: `GET /v1/mods/{run_id}/diffs` lists diffs for a run.
  - v1: list diffs under `/v1/runs/{run_id}/repos/{repo_id}/diffs` (repo-scoped).
  - v0 server note: there is no `GET /v1/runs/{run_id}/diffs` route registered today; list is under `/v1/mods/{run_id}/diffs`.
  - v0: `GET /v1/diffs/{diff_id}?download=true` downloads a diff.
  - v1: download may remain global by `diff_id` (repo scoping comes from the listing endpoint).
- v0 run logs/events streaming:
  - v0: `GET /v1/runs/{run_id}/logs` is an SSE stream of logs + events for a run.
  - v1: add `/v1/runs/{run_id}/repos/{repo_id}/logs` SSE to stream only logs/events for jobs in that repo.
