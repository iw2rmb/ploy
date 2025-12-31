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
- Prints `mod_id` and name; if `--spec` is provided, also prints `spec_id`.

### `ploy mod list`

- Lists mods: `ID`, `NAME`, `CREATED_AT`, `ARCHIVED_AT`, optional `DEFAULT_SPEC`.

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
- Returns `spec_id`.

### `ploy mod spec list <mod-id|name>`

- Lists spec variants for the mod: `ID`, `NAME`, `CREATED_AT`, `ARCHIVED`.

### `ploy mod spec remove <mod-id|name> <spec-id|name>`

- Archives or deletes a spec variant.

### `ploy mod spec default <mod-id|name> <spec-id|name>`

- Sets the mod’s default spec used by `ploy mod run` when `--spec` is omitted.

## Repo set management

### `ploy mod repo add <mod-id|name> --repo <repo-url> --base-ref <ref> --target-ref <ref>`

- Adds a repo to the mod repo set (identity is `repo_url` within a mod).
- Returns `mod_repo_id`.

### `ploy mod repo list <mod-id|name>`

- Lists repos in the mod: `ID`, `REPO_URL`, `BASE_REF`, `TARGET_REF`, `ADDED_AT`.

### `ploy mod repo remove <mod-id|name> --repo-id <mod_repo_id> [--force]`

- Deletes the repo row from the mod’s repo set.
- If there are historical executions referencing this repo (`run_repos.mod_repo_id` exists), require `--force`:
  - without `--force`: return a clear error
  - with `--force`: delete the row and preserve history via FK `ON DELETE SET NULL`

### `ploy mod repo import <mod-id|name> --file <path>`

- Imports repos in bulk from CSV.
- CSV columns: `repo_url,base_ref,target_ref` (header required).
- Creates/updates repos; reports counts and per-line errors.

## Run execution

### `ploy mod run <mod-id|name> [--spec <id|name>] [--repo <repo-url> ...] [--failed]`

Behavior:

- Resolves `<mod-id|name>` to a mod.
- Refuses when the mod is archived.
- Resolves the chosen spec:
  - `--spec <id|name>` → stored spec variant
  - omitted → mod default spec (error if none)
- Selects repos:
  - `--repo ...` → explicit repos (by repo_url identity within the mod)
  - `--failed` → repos with last terminal state `failed`
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
  - If the associated mod repo `base_ref` currently resolves to a different commit SHA than the run’s recorded base commit SHA:
    - if the mod is archived: error
    - else: run the mod for this repo and pull the new run’s diffs instead

### `ploy run diff [--repo <repo>] <run-id>`

Show the aggregated diff for a run, optionally scoped to a single repo within a multi-repo run.

Repo selection when `--repo` is omitted:

- If the run has exactly one repo: show diff for that repo.
- Else if invoked from a git worktree whose `origin` URL matches a repo in the run: show diff for that repo.
- Else: error (repo must be specified).

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
  - If the mod has never been executed for this repo **or** the mod repo `base_ref` currently resolves to a different commit SHA than the selected run’s recorded base commit SHA:
    - if the mod is archived: error
    - else: run the mod for this repo and pull the new run’s diffs instead

## Name/ID resolution rules

- `<mod-id|name>`: prefer exact ID match; else unique name match; else error with suggestions.
- `<spec-id|name>`: scoped to the mod; prefer ID match; else unique name match.
