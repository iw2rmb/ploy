# Roadmap v1 — API

## Mod projects

### `POST /v1/mods`

Creates a mod project.

Request:

- `name` (string, unique)
- optional `spec` (object; JSON) — creates an initial spec variant

Response:

- `id`, `name`, `created_at`, optional `spec_id` (when `spec` is provided)

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

- `name` (string, unique within mod)
- `spec` (object; JSON)

Response:

- `id`, `name`, `created_at`

### `GET /v1/mods/{mod_id}/specs`

Lists spec variants for a mod.

### `DELETE /v1/mods/{mod_id}/specs/{spec_id}`

Archives or deletes a spec variant.

### `POST /v1/mods/{mod_id}/specs/{spec_id}/default`

Sets mod default spec.

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

### `DELETE /v1/mods/{mod_id}/repos/{mod_repo_id}?force=true`

Deletes a repo from the mod repo set.

- If the repo has historical executions (referenced by `run_repos.mod_repo_id`), require `force=true`.
- Without `force=true`, return a clear error explaining that the repo has run history.

## Running a mod

### `POST /v1/mods/{mod_id}/runs`

Creates a batch run from the mod + spec + selected repos and immediately starts it.

This is the API behind `ploy mod run <mod> ...`.

Request (suggested):

- `spec_ref`:
  - `{ "id": "<spec_id>" }` or `{ "name": "<spec_name>" }`
- `repo_selector`:
  - `{ "mode": "all" }`
  - `{ "mode": "failed" }`
  - `{ "mode": "explicit", "repos": ["<repo_url>", ...] }`
- optional per-run overrides:
  - `created_by`
  - optional ref overrides when `mode=explicit` (exact shape TBD)

Response:

- `run_id`
- `started`, `already_done`, `pending`

## Schema/docs updates

- Add OpenAPI schemas for `Mod`, `ModSpec`, `ModRepo`, `CreateModRunRequest`, `CreateModRunResponse`.
- Keep existing `/v1/runs/*` APIs as the run execution/history surface; mod APIs are just project/spec/repo management + run creation.
