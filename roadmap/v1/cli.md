# Roadmap v1 — CLI

## Mod management

### `ploy mod add --name <name>`

- Creates a mod with unique `<name>`.
- Prints `mod_id` and name.

### `ploy mod list`

- Lists mods: `ID`, `NAME`, `CREATED_AT`, optional `DEFAULT_SPEC`.

### `ploy mod remove <mod-id|name>`

- Archives (preferred) or deletes a mod and detaches future runs.

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

### `ploy mod repo add <mod-id|name> --repo-url <url> --base-ref <ref> --target-ref <ref>`

- Adds or re-enables a repo in the mod repo set.
- Returns `mod_repo_id`.

### `ploy mod repo list <mod-id|name>`

- Lists repos in the mod: `ID`, `REPO_URL`, `BASE_REF`, `TARGET_REF`, `ENABLED`, `ADDED_AT`, `REMOVED_AT`.

### `ploy mod repo remove <mod-id|name> --repo-id <mod_repo_id> [--force]`

- Removes the repo row from the mod’s repo set.
- If there are historical executions referencing this repo (`run_repos.mod_repo_id` exists), require `--force`:
  - without `--force`: return a clear error
  - with `--force`: delete the row and preserve history via FK `ON DELETE SET NULL`

### `ploy mod repo import <mod-id|name> --file <path>`

- Imports repos in bulk from CSV.
- CSV columns: `repo_url,base_ref,target_ref` (header required).
- Creates/updates repos; reports counts and per-line errors.

## Run execution

### `ploy mod run <mod-id|name> [--spec <id|name|path>] [--repo-id <id> ...] [--failed] [--repo-url/base-ref/target-ref overrides]`

Behavior:

- Resolves `<mod-id|name>` to a mod.
- Resolves the chosen spec:
  - `--spec <id|name>` → stored spec variant
  - `--spec <path>` → inline spec (optionally persisted later)
  - omitted → mod default spec (error if none)
- Selects repos:
  - `--repo-id ...` → explicit repos
  - `--failed` → repos with last terminal state `failed`
  - omitted → all enabled repos
- Creates a batch run and immediately starts execution.
- Prints:
  - created `run_id`
  - counts: started/already done/pending (matching existing `run start` summary)

## Name/ID resolution rules

- `<mod-id|name>`: prefer exact ID match; else unique name match; else error with suggestions.
- `<spec-id|name>`: scoped to the mod; prefer ID match; else unique name match.
