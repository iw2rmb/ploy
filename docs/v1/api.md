# Roadmap v1 — API

## Mod projects

### `POST /v1/mods`

Creates a mod project.

Request:

- `name` (string, unique)
- optional `spec` (object; JSON) — creates an initial spec variant

Response:

- `id`, `name`, `created_at`, optional `spec_id` (when `spec` is provided)

Behavior:

- If `spec` is provided, set `mods.spec_id` to the created `spec_id`.

### `GET /v1/mods`

Lists mods.

Query:

- `limit`, `offset`
- optional filters: `name_substring`, `archived` (exact set TBD)

### `DELETE /v1/mods/{mod_id}`

Deletes a mod.

### `PATCH /v1/mods/{mod_id}/archive`

Archives a mod.

- When a mod is archived, it cannot be executed (any attempt to create a mod run must fail).
- A mod cannot be archived while it has any running jobs.

### `PATCH /v1/mods/{mod_id}/unarchive`

Unarchives a mod.

## Single-repo runs

### `POST /v1/runs`

Submits a single-repo run and immediately starts execution.

This is the API behind `ploy run --spec ... --repo ...`.

Side-effects:

- Creates a mod project; the created mod has `name == id`.
- Creates an initial spec variant for that mod from the provided `spec`.
- Creates a mod repo row for the provided `repo_url` (identity within the mod).

Request (suggested):

- `repo_url`
- `base_ref`
- `target_ref`
- `spec` (object; JSON)

Response:

- `run_id`
- `mod_id`
- `spec_id`

## Spec variants

### `POST /v1/mods/{mod_id}/specs`

Creates a spec variant for a mod.

Request:

- `name` (string)
- `spec` (object; JSON)

Response:

- `id`, `name`, `created_at`

Behavior:

- Update `mods.spec_id` to the created `spec_id`.

### `GET /v1/mods/{mod_id}/specs`

Lists spec variants for a mod.

Notes:

- Returned set includes only specs that were actually used in at least one run:
  - distinct `runs.spec_id` values where `runs.mod_id == mod_id`

### `DELETE /v1/mods/{mod_id}/specs/{spec_id}`

Archives or deletes a spec variant.

Behavior:

- If `spec_id` is referenced by any `runs.spec_id`: set `specs.archived_at` (do not hard-delete).
- Else: delete the `specs` row.

## Repo set

### `POST /v1/mods/{mod_id}/repos`

Adds/enables a repo in a mod.

Request:

- `repo_url`
- `base_ref`
- `target_ref`

Response:

- `id` (mod_repo_id), plus stored fields.

### `GET /v1/mods/{mod_id}/repos`

Lists repos for a mod.

### `DELETE /v1/mods/{mod_id}/repos/{mod_repo_id}`

Deletes a repo from the mod repo set.

- Refuse deletion if the repo has historical executions (referenced by `run_repos.repo_id`).

## Running a mod

### `POST /v1/mods/{mod_id}/runs`

Creates a batch run from the mod + spec + selected repos and immediately starts it.

This is the API behind `ploy mod run <mod> ...`.

Request (suggested):

- optional `spec_ref`:
  - `{ "id": "<spec_id>" }`
- `repo_selector`:
  - `{ "mode": "all" }`
  - `{ "mode": "failed" }` (repos whose last terminal state is `Failed`)
  - `{ "mode": "explicit", "repos": ["<repo_url>", ...] }`
- optional per-run overrides:
  - `created_by`
  - optional ref overrides when `mode=explicit` (exact shape TBD)

Behavior:

- If `spec_ref` is omitted, use `mods.spec_id`; if `mods.spec_id` is NULL, return an error that spec is required.
- If `spec_ref` is provided, it may reference any `specs.id` (not restricted to the mod’s current spec id).
- If `spec_ref` is provided, update `mods.spec_id` to the resolved `spec_id` for future runs.

Response:

- `run_id`
- `started`, `already_done`, `pending`

## Schema/docs updates

- Add OpenAPI schemas for `Mod`, `Spec`, `ModRepo`, `CreateModRunRequest`, `CreateModRunResponse`.
- Move run-scoped “artifacts” endpoints that currently live under `/v1/mods/{run_id}/*` to a run-scoped namespace to avoid colliding with `/v1/mods` (mod projects).
- For multi-repo runs, repo-scoped artifacts are addressed under `/v1/runs/{run_id}/repos/{repo_id}/...` where `repo_id` is `mod_repos.id` (aka `mod_repo_id`).
- Repo-scoped endpoints required for CLI workflows (names TBD, but must exist under the repo-scoped namespace):
  - diffs: list + download
  - events: list/stream (used for progress/diagnostics)
  - logs: list/stream (scoped by `job_id`, but repo attribution comes from `jobs.repo_id`)
  - artifacts: list + download (job-produced bundles)
- Keep existing `/v1/runs/*` APIs as the run execution/history surface; mod APIs are just project/spec/repo management + run creation.
