# Roadmap v1 — CLI

## Run submission (single-repo)

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
- If `--spec` is provided, also creates an initial spec variant for the mod.
- If `--spec` is provided, sets `mods.spec_id` to the created `spec_id`.
- Prints `mod_id` and name; if `--spec` is provided, also prints `spec_id`.

### `ploy mod list`

- Lists mods: `ID`, `NAME`, `CREATED_AT`, `ARCHIVED_AT`.

### `ploy mod remove <mod-id|name>`

- Deletes a mod.

### `ploy mod archive <mod-id|name>`

- Archives a mod.
- Refuses when any jobs for that mod are currently running.

### `ploy mod unarchive <mod-id|name>`

- Unarchives a mod.

## Spec variants

Mod is always a required positional argument for spec management.

### `ploy mod spec add --name <spec-name> <mod-id|name> <path|->`

Suggested concrete shape:

- `ploy mod spec add --name <spec-name> <mod-id|name> <path>`
- `ploy mod spec add --name <spec-name> <mod-id|name> -` (read spec from stdin)

Behavior:

- Stores the parsed spec JSON (from YAML/JSON input).
- Validates spec shape.
- Inserts a new `specs` row and updates `mods.spec_id` to that new `spec_id`.
- Returns `spec_id`.

### `ploy mod spec list <mod-id|name>`

- Lists spec variants for the mod: `ID`, `NAME`, `CREATED_AT`, `ARCHIVED`.
- Returned set includes only specs that were actually used in at least one run for this mod.

### `ploy mod spec remove <mod-id|name> <spec-id>`

- Archives or deletes a spec variant.
- If `<spec-id>` is referenced by any `runs.spec_id`: archive (set `specs.archived_at`).
- Else: delete the `specs` row.

## Repo set management

### `ploy mod repo add <mod-id|name> --repo <repo-url> --base-ref <ref> --target-ref <ref>`

- Adds a repo to the mod repo set (identity is `repo_url` within a mod).
- Returns `mod_repo_id`.

### `ploy mod repo list <mod-id|name>`

- Lists repos in the mod: `ID`, `REPO_URL`, `BASE_REF`, `TARGET_REF`, `ADDED_AT`.

### `ploy mod repo remove <mod-id|name> --repo-id <mod_repo_id>`

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
- Upserts by `repo_url` within the mod (updates `base_ref`/`target_ref` for existing rows).
- Does not affect historical run data (existing `run_repos.repo_base_ref` / `run_repos.repo_target_ref` snapshots remain unchanged).
- Continues on errors; may partially apply; reports counts and per-line errors.

## Run execution

### `ploy mod run <mod-id|name> [--spec <id>] [--repo <repo-url> ...] [--failed]`

Behavior:

- Resolves `<mod-id|name>` to a mod.
- Refuses when the mod is archived.
- Resolves the chosen spec:
  - `--spec <id>` → set as the mod’s current spec (`mods.spec_id`)
  - omitted → use `mods.spec_id` (error if NULL)
- Selects repos:
  - `--repo ...` → explicit repos (by repo_url identity within the mod)
  - `--failed` → repos with last terminal state `Failed`
  - omitted → all repos in the mod repo set
- Creates a mod-scoped run via `POST /v1/mods/{mod_id}/runs` and immediately starts execution.
- Prints:
  - created `run_id`
  - counts: started/already done/pending (matching existing `run start` summary)

## Pulling diffs locally

### `ploy run pull <run-id>`

Pull Mods-generated diffs into the current git worktree.

Behavior:

- Pulls diffs for the specified `run-id`.
- Safety check against base ref drift:
  - (requires v1 decision on where “recorded base commit SHA” lives; see docs/v1/docs.md)
  - If the associated mod repo `base_ref` currently resolves to a different commit SHA than the run’s recorded base commit SHA:
    - if the mod is archived: error
    - else: run the mod for this repo and pull the new run’s diffs instead

### `ploy run diff [--repo <repo>] <run-id>`

Show the aggregated diff for a run, optionally scoped to a single repo within a multi-repo run.

Repo selection when `--repo` is omitted:

- If the run has exactly one repo: show diff for that repo.
- Else if invoked from a git worktree whose `origin` URL matches a repo in the run: show diff for that repo.
- Else: error (repo must be specified).

## Run control

### `ploy run stop <run-id> [--repo <repo-id>]`

- Stops a run (`runs.status` → `Stopped`).
- When `--repo <repo-id>` is provided, stops that repo in the run (`run_repos.status` → `Stopped`).
  - `<repo-id>` is `mod_repos.id` (aka `mod_repo_id`).

### `ploy run start <run-id>`

- Starts or resumes a run (`runs.status` → `Started`).

### `ploy mod pull [--last | --last-failed | --last-succeeded] [<mod-name|id>]`

Behavior:

- Selects a run relative to a mod project and pulls its diffs into the current git worktree.
- If `<mod-name|id>` is provided, it is used to select the mod.
- If `<mod-name|id>` is omitted, the CLI infers the mod from the current repository:
  - Find all **non-archived** mods that include this repo URL in their repo set.
  - If exactly one mod matches: select that mod.
  - If multiple mods match: error and print the matching mods (IDs + names).
- Default (no selector flags): behave as `--last-succeeded`.
- Safety check against base ref drift / missing execution:
  - (requires v1 decision on where “recorded base commit SHA” lives; see docs/v1/docs.md)
  - If the mod has never been executed for this repo **or** the mod repo `base_ref` currently resolves to a different commit SHA than the selected run’s recorded base commit SHA:
    - if the mod is archived: error
    - else: run the mod for this repo and pull the new run’s diffs instead

Notes:

- `ploy mod pull` is executed from a repo folder and selects diffs for the current repo by looking up the matching `run_repos` entry in the chosen run.

## Name/ID resolution rules

- `<mod-id|name>`: prefer exact ID match; else unique name match; else error with suggestions.
- `<spec-id>`: must be a spec ID.
