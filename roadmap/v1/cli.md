# Roadmap v1 — CLI

## Run submission (single-repo)

Change entry: move single-repo submission from `ploy mod run` to `ploy run`.

- Current (HEAD): single-repo run submission uses `ploy mod run ...` and calls `POST /v1/runs` (see `internal/server/handlers/register.go`, and CLI docs in `docs/mods-lifecycle.md`).
- Current (HEAD): `ploy run --repo ... --base-ref ... --target-ref ... --spec ...` calls `POST /v1/runs` (see `roadmap/v1/api.md`).
- Where: CLI routing under `cmd/ploy/*` and HTTP client under `internal/cli/*`.
- Compatibility: breaking CLI/API surface; no backward compatibility required.
- Unchanged: existing run lifecycle surfaces under `ploy run ...` (list/status/cancel/logs) remain, with repo scoping added for multi-repo runs.

### `ploy run --repo <repo-url> --base-ref <ref> --target-ref <ref> --spec <path|->`

- Submits a single-repo run and immediately starts execution.
- Uses `POST /v1/runs`.
- Creates a mod project as a side-effect; the created mod has `name == id`.
- Prints `run_id` and `mod_id`.

Notes:
- `--spec -` reads the spec from stdin.
- Use `ploy mod run <mod>` to re-run the created mod project.

## Mod management

### `ploy mod add --name <name> [--spec <path|->]`

- Creates a mod with unique `<name>`.
- If `--spec` is provided, also creates an initial spec row for the mod.
- If `--spec` is provided, sets `mods.spec_id` to the created `spec_id`.
- Prints `mod_id` and name; if `--spec` is provided, also prints `spec_id`.

### `ploy mod list`

- Lists mods: `ID`, `NAME`, `CREATED_AT`, `ARCHIVED_AT`.

### `ploy mod remove <mod-id|name>`

- Deletes a mod.
- Refuses if the mod has any runs.

### `ploy mod archive <mod-id|name>`

- Archives a mod.
- Refuses when any jobs for that mod are currently running.

### `ploy mod unarchive <mod-id|name>`

- Unarchives a mod.

## Setting a mod spec

### `ploy mod spec set <mod-id|name> <path|->`

Behavior:

- Stores the parsed spec JSON (from YAML/JSON input).
- Validates spec shape.
- Inserts a new `specs` row and updates `mods.spec_id` to that new `spec_id`.
- Returns `spec_id`.

## Repo set management

Change entry: `ploy mod repo import` CSV parsing is fully specified.

- Current (HEAD): no bulk import command exists for mod repo sets.
- Proposed (v1): `ploy mod repo import` parses strict CSV with comma delimiter, optional quoting, unicode support, and continues on per-line errors.
- Where: CLI parsing/validation under `cmd/ploy/*` + server endpoint under `internal/server/handlers/*`.
- Compatibility: new feature; no backward compatibility required.
- Unchanged: repo identity remains `(mod_id, repo_url)` and imports rewrite refs in-place.

### `ploy mod repo add <mod-id|name> --repo <repo-url> --base-ref <ref> --target-ref <ref>`

- Adds a repo to the mod repo set (identity is `repo_url` within a mod).
- Returns `repo_id` (see `roadmap/v1/db.md` “Identifier conventions (v1)”).

### `ploy mod repo list <mod-id|name>`

- Lists repos in the mod: `ID`, `REPO_URL`, `BASE_REF`, `TARGET_REF`, `ADDED_AT`.

### `ploy mod repo remove <mod-id|name> --repo-id <repo_id>`

- Deletes the repo row from the mod’s repo set.
- Refuse deletion if there are historical executions referencing this repo (`run_repos.repo_id` exists).

### `ploy mod repo import <mod-id|name> --file <path>`

- Imports repos in bulk from CSV.
- CSV columns: `repo_url,base_ref,target_ref` (header required).
- Parsing:
  - delimiter: `,`
  - UTF-8 text; unicode characters allowed
  - fields may be quoted with `"` (CSV-style)
  - within quoted fields, `"` is escaped as `""`
- Calls `POST /v1/mods/{mod_id}/repos/bulk` with `Content-Type: text/csv`.
- Upserts by `repo_url` within the mod (updates `base_ref`/`target_ref` for existing rows).
- Does not affect historical run data (existing `run_repos.repo_base_ref` / `run_repos.repo_target_ref` snapshots remain unchanged).
- Continues on errors; may partially apply; reports counts and per-line errors.

## Run execution

### `ploy mod run <mod-id|name> [--repo <repo-url> ...] [--failed]`

Behavior:

- Resolves `<mod-id|name>` to a mod.
- Refuses when the mod is archived.
- Uses `mods.spec_id` as the run’s `spec_id` (copied onto the created run).
- Spec changes are mod-scoped only:
  - `ploy mod spec set <mod> <path|->` updates `mods.spec_id`
  - `ploy mod run <mod>` always uses the current `mods.spec_id` (error if NULL)
- Selects repos:
  - `--repo ...` → explicit repos (by repo_url identity within the mod)
  - `--failed` → repos with last terminal state `Fail`
  - omitted → all repos in the mod repo set
- Creates a mod-scoped run via `POST /v1/mods/{mod_id}/runs` and immediately starts execution.
- Prints:
  - created `run_id`
  - prints only `run_id` (run starts immediately; no separate “start” operation)

## Pulling diffs locally

### `ploy run pull <run-id>`

Pull Mods-generated diffs into the current git worktree.

v0 reference:

- Older designs used per-repo execution runs (`run_repos.execution_run_id`).
- v1 removes per-repo execution runs; pull is based on the run ID plus repo context (repo resolution is done via repo-centric APIs; artifacts are fetched via run/job IDs).

Behavior:

- `ploy run pull` is executed from inside a repo folder; the CLI derives the current repo identity from the configured git remote URL (`origin` by default).
- Resolve repo execution for this run on the server:
  - Call `POST /v1/runs/{run_id}/pull` with `repo_url` for the current repo (see `roadmap/v1/scope.md` “Repo URL rules (v1)”).
  - Server returns `repo_id` and `repo_target_ref`.
- Pull diffs for `(run_id, repo_id)` via `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` and download via `GET /v1/diffs/{diff_id}?download=true`.

## Run control

### `ploy run cancel <run-id>`

- Cancels a run (`runs.status` → `Cancelled`).

### `ploy mod pull [--last-failed | --last-succeeded] [<mod-name|id>]`

Behavior:

- Selects a run relative to a mod project and pulls its diffs into the current git worktree.
- If `<mod-name|id>` is provided, it is used to select the mod.
- If `<mod-name|id>` is omitted, the CLI infers the mod from the current repository:
  - Call `GET /v1/mods?repo_url=<current_repo_url>&archived=false` to find candidate mods that include this repo URL in their repo set.
  - Find all **non-archived** mods that include this repo URL in their repo set.
  - If exactly one mod matches: select that mod.
  - If multiple mods match: error and print the matching mods (IDs + names).
- Call `POST /v1/mods/{mod_id}/pull` with:
  - `repo_url` for the current repo (see `roadmap/v1/scope.md` “Repo URL rules (v1)”),
  - and the selected mode (`last-succeeded` default, or `last-failed`).
  - Server returns `run_id`, `repo_id`, `repo_target_ref` (see `roadmap/v1/api.md`).
- Pull diffs for `(run_id, repo_id)` via `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` and download via `GET /v1/diffs/{diff_id}?download=true`.

Notes:

- `ploy mod pull` is executed from a repo folder and selects diffs for the current repo by looking up the matching `run_repos` entry in the chosen run.
- Repo URL matching must follow `roadmap/v1/scope.md` (“Repo URL rules (v1)”).

## Name/ID resolution rules

- `<mod-id|name>`: prefer exact ID match; else unique name match; else error with suggestions.
- `<spec-id>`: must be a spec ID.
