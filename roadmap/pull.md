# Roadmap: `ploy pull` (local repo pull workflow)

## Goal

Add a new top-level CLI command `ploy pull` that:
- ensures a Mods run exists for the **current local repo HEAD SHA** (recorded at initiation time),
- and then pulls the resulting diffs into the local git worktree (via the existing pull mechanics).

This roadmap is for future work. It documents intended behavior at HEAD and what will change.

## Current state (HEAD)

There is no top-level `ploy pull` command.

Related existing commands:
- `ploy run pull <run-id>`: pulls diffs from a specific run into the current repo (`cmd/ploy/run_pull.go` + git helpers in `cmd/ploy/pull_helpers.go`).
- `ploy mod pull [<mod>]`: resolves a run+repo via `POST /v1/mods/{mod_id}/pull` (last succeeded/failed) and pulls diffs (`cmd/ploy/mod_pull.go`).

Relevant server endpoints:
- `POST /v1/runs/{run_id}/pull`: resolve repo execution identifiers for a given run and `repo_url` (`internal/server/handlers/pull.go`).
- `POST /v1/mods/{mod_id}/pull`: resolve latest run for a mod by `repo_url` and mode (`internal/server/handlers/pull.go`).
- `GET /v1/runs/{id}/status`: canonical Mods-style run status (`internal/server/handlers/mods_ticket.go`).
- `GET /v1/runs/{id}/repos`: list repos in a run (batch model) (`internal/server/handlers/runs_batch_http.go`).

Docs describing existing pull behavior:
- `docs/mods-lifecycle.md` → “Pulling Diffs Locally (`run pull` / `mod pull`)”
- `cmd/ploy/README.md` → “Pull Mods Changes Locally”

## Proposed command

### Surface

New command:
- `ploy pull [flags]`

New flags (v0):
- `--new-run`: force initiating a new run and overwriting the local pull state.
- `--follow`: follow run progress until terminal (new meaning; see `roadmap/follow.md`).

Reused flags (consistent with existing pull commands):
- `--origin <remote>`: git remote to match (default `origin`) (matches `cmd/ploy/run_pull.go`, `cmd/ploy/mod_pull.go`).
- `--dry-run`: validate and print actions without mutating the repo (matches `cmd/ploy/run_pull.go`, `cmd/ploy/mod_pull.go`).

Non-goals (v0):
- No “last-failed/last-succeeded” selection. `ploy pull` is HEAD-SHA keyed, not “latest run” keyed.
- No compatibility behavior for the current `--follow` meaning (log streaming). Use `ploy run logs` (and the new follow behavior in `roadmap/follow.md`).

### Behavior

Preconditions:
- Must be run inside a git repository (`ensureInsideGitWorktree` in `cmd/ploy/pull_helpers.go`).
- Must have a clean working tree before mutating (same as existing pull commands via `ensureCleanWorkingTree`).
- Must be able to resolve a repo identity from a git remote URL (default `origin`) (`resolveGitRemoteURL` in `cmd/ploy/pull_helpers.go`).

Run selection and SHA policy:
- Determine local `HEAD` SHA at invocation time (via `git rev-parse HEAD`).
- Maintain a per-repo “pull state” record that binds:
  - `repo_url` (derived from remote),
  - `head_sha` (local HEAD at run initiation),
  - `run_id` (server-assigned run id),
  - `created_at` (timestamp, optional but useful for messaging).

Decision rules:
1) If there is **no saved pull state**, `ploy pull` MUST initiate a run:
   - create a run for the inferred mod project scoped to the current repo URL (see “Run initiation” below),
   - persist `{repo_url, head_sha, run_id, created_at}`,
   - then:
     - if `--follow`: follow it to terminal and proceed to pull diffs,
     - else: return an error that clearly says the run was initiated and the user should rerun with `--follow` (or inspect status) before diffs can be pulled.

2) If there is saved pull state and `state.head_sha != current HEAD SHA`:
   - return an error that MUST mention `--new-run` (verbatim) as the remedy.
   - Do not mutate any local branches/worktree.

3) If there is saved pull state and `state.head_sha == current HEAD SHA`:
   - reuse `state.run_id`.
   - If the run is not terminal and `--follow` is not set: return an error telling the user to rerun with `--follow`.
   - If `--follow` is set: follow until terminal; on success proceed to pull diffs.

4) If `--new-run` is set:
   - always initiate a new run (even if SHA matches),
   - overwrite pull state with the new `{head_sha, run_id}`.

Run initiation (server-side identity):
- Infer the mod project from the current repo URL using the same inference logic as `cmd/ploy/mod_pull.go` (`inferModFromRepo` calls `GET /v1/mods?repo_url=...`).
- Create a mod-project run scoped to only this repo via `POST /v1/mods/{mod_id}/runs` with `repo_selector.mode="explicit"` and `repos=[repo_url]` (client exists at `internal/cli/mods/mod_run.go`).

Pulling diffs:
- Once a run is terminal-success, reuse the existing `ploy run pull <run-id>` logic to apply diffs:
  - `POST /v1/runs/{run_id}/pull` to resolve `(repo_id, repo_target_ref)` for the current repo URL (`cmd/ploy/run_pull.go` → `internal/cli/mods.RunPullCommand`),
  - fetch `base_ref` for checkout (`fetchRunRepoDetails` in `cmd/ploy/run_pull.go`),
  - shallow fetch `base_ref` from remote and create a branch at that commit (`cmd/ploy/pull_helpers.go`),
  - download and apply diffs (`downloadAndApplyDiffs`).

### Local pull state storage

Store location (proposal):
- In the git directory, not the worktree, to avoid accidental commits:
  - path: `<git-dir>/ploy/pull_state.json`
  - resolve `<git-dir>` with `git rev-parse --git-dir`.

Schema (proposal):
```json
{
  "repo_url": "https://github.com/org/repo.git",
  "head_sha": "0123abcd...",
  "run_id": "2M7x...KSUID...",
  "created_at": "2026-01-15T00:00:00Z"
}
```

## Error messages (requirements)

When SHA mismatch is detected:
- Must mention `--new-run` explicitly.
- Must include the current HEAD SHA and the stored head SHA.

Example shape (not exact wording):
- `pull: current HEAD <sha1> does not match saved run HEAD <sha2>; rerun with --new-run to initiate a new run`

## Implementation sketch (paths/symbols)

CLI:
- Add a new cobra command `pull` under `cmd/ploy/root.go` via a `newPullCmd` builder (pattern: `cmd/ploy/commands_mod.go` / `newRunCmd`).
- Implement handler in a new file, e.g. `cmd/ploy/pull.go`, using helpers from:
  - `cmd/ploy/pull_helpers.go`
  - `cmd/ploy/mod_pull.go` (reuse `inferModFromRepo` or extract shared logic)
  - `internal/cli/mods/mod_run.go` (create mod-project run)
  - `cmd/ploy/run_pull.go` (apply diffs by run id)

Local state helpers (new):
- read/write `pull_state.json` under `<git-dir>/ploy/`.

## What remains unchanged

- `ploy run pull <run-id>` and `ploy mod pull ...` continue to exist and keep their core semantics (pull diffs from an already-selected run).
- Log streaming remains available via `ploy run logs ...` (see `cmd/ploy/run_commands.go` and `internal/cli/mods/logs.go`).

## Docs to update after implementation

- `cmd/ploy/README.md`: add `ploy pull` usage and clarify how it relates to `run pull` / `mod pull`.
- `docs/mods-lifecycle.md`: add `ploy pull` to the “Pulling Diffs Locally” section and update any mention of `--follow` under CLI surfaces to match the new meaning.
- `docs/api/OpenAPI.yaml`: if `ploy pull` requires any new control-plane endpoints beyond existing `GET /v1/mods?repo_url=...` and `POST /v1/mods/{mod_id}/runs`, update OpenAPI accordingly (and add new `docs/api/paths/*.yaml` if needed).
- `tests/e2e/mods/README.md`: update references to `--follow` meaning (log streaming → graph follow).
