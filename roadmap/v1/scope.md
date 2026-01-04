# Roadmap v1 — Scope

## Goal

Support “code modification projects” where:

- A **mod** is a long-lived project with a unique name.
- A mod has a single **current spec** (Mods YAML/JSON), referenced by `mods.spec_id`.
  - Setting/updating a mod spec creates a new `specs` row and updates `mods.spec_id`.
- A mod has a managed **repo set** (identified by `repo_url`) that changes over time.
- A **run** executes the mod’s current spec (copied from `mods.spec_id`) over:
  - one repo,
  - a selected subset of repos,
  - or the mod’s repos whose last run state is `Fail`.

Run entrypoints:

- `ploy run --spec ... --repo ...` creates a run and immediately starts execution (single-repo). It also creates a mod project as a side-effect; the created mod has `name == id`.
- `ploy mod run <mod> ...` creates a run for a mod project and immediately starts execution over the mod’s repo set.

## Terms (no new nouns)

- **Mod**: project container (unique name).
- **Spec**: the current Mods spec referenced by a mod (`mods.spec_id` → `specs.id`).
- **Repo**: a repo participating in a mod (repo_url + refs).
- **Run**: an execution attempt; produces run-level and per-repo status, artifacts, logs, diffs.

## Repo URL rules (v1)

- Validation: accept only `https://`, `ssh://`, and `file://` (see `internal/domain/types/vcs.go`, `RepoURL.Validate`).
- Normalization for comparisons: use `internal/worker/hydration/git_fetcher.go` `normalizeRepoURL` (strip trailing `/` and `.git`).
  - Reuse this helper for all server-side `repo_url` matching; do not re-implement per endpoint.

## Non-goals (v1)

- Cross-mod spec sharing.
- Automatic repo discovery from orgs/monorepos.
- Scoring frameworks beyond storing basic metrics + optional human score.
- Backward compatibility layers or migrations for legacy “runs-only” workflows.

## Key behaviors

- **Immutability**: a run links to the exact spec used (`runs.spec_id`).
- **Stable grouping**: grouping is by `mods.name` (unique) and `runs.mod_id` (no `runs.name`).
- **Archiving**: archived mods cannot be executed.
- **Repo refs over time**:
  - `mod_repos` rows are mutable (e.g., CSV import can rewrite refs).
  - `run_repos.repo_base_ref` and `run_repos.repo_target_ref` snapshot refs used for that repo in that run.
- **Repo selection**:
  - CLI and API details: see `roadmap/v1/cli.md` (`ploy mod run`) and `roadmap/v1/api.md` (`POST /v1/mods/{mod_id}/runs`).
- **Immediate start**: both `ploy run` and `ploy mod run` start queued work right away.

## Execution model shift (required)

Codebase must switch from “root-run → per-repo execution runs” to “run → run_repos”.

### Job claiming vs repo progression (v1 invariant)

- Claiming stays global: nodes call `POST /v1/nodes/{id}/claim` with no repo selector.
- Progression rules (promotion + attempt scoping): see `roadmap/v1/statuses.md` “Job queueing rules (v1)” and `roadmap/v1/db.md` (`jobs.repo_id`, `jobs.attempt`).

## `/v1/mods/*` route collisions (v0 reference)

Current server routes under `/v1/mods/*` are run-scoped (see `internal/server/handlers/register.go`) and collide with v1’s “mods are projects” direction.

- `POST /v1/mods` (run submission) must move to `POST /v1/runs`.
- Run lifecycle routes under `/v1/mods/{run_id}/*` must move under `/v1/runs/{run_id}/*`:
  - example: `POST /v1/mods/{run_id}/cancel` → `POST /v1/runs/{run_id}/cancel`.
  - v0 `POST /v1/mods/{run_id}/resume` is removed in v1; use repo-level `POST /v1/runs/{run_id}/repos/{repo_id}/restart` instead (see `roadmap/v1/api.md`).
- Repo-specific artifacts must move under the repo-scoped namespace:
  - example: `GET /v1/mods/{run_id}/diffs` → `GET /v1/runs/{run_id}/repos/{repo_id}/diffs`
- Endpoint rename: `POST /v1/runs/{run_id}/stop` becomes `POST /v1/runs/{run_id}/cancel`.

Current codebase behavior (to remove):

- `runs` row acts as a batch root and stores repo_url/refs/spec.
- Each `run_repos` row may point to a separate execution run via `run_repos.execution_run_id` (this linkage is removed in v1).
- Jobs, logs, diffs, and events are attached to the execution run ID, not the `(run_id, repo_id)` pair.

v1 behavior (to implement):

- One `runs` row represents the run (mod + spec) and holds `runs.mod_id` + `runs.spec_id`.
- Per-repo execution state is represented by `run_repos` rows (scoped to `runs.id`).
- Jobs become repo-scoped by adding `jobs.repo_id` and `jobs.repo_base_ref` (copied from `run_repos`).
- Logs/diffs/events remain addressed by `run_id`, but repo attribution comes from `job_id → jobs.repo_id`.

Code pointers for the refactor:

- `internal/server/handlers/mods_ticket.go`: `submitRunHandler` currently creates a parent run, inserts `run_repos`, then relies on `StartPendingRepos` which expects `execution_run_id`.
- `internal/server/handlers/runs_batch_scheduler.go`: currently creates a child run per repo, calls `createJobsFromSpec` on the child run, and stores `execution_run_id`.
- `internal/server/handlers/nodes_complete_run.go`: completion logic derives run status by listing jobs for a run ID; in v1, completion must also update `run_repos.status` per repo.
- `internal/server/handlers/repos.go`: repo-centric endpoints currently expose `execution_run_id`; v1 must remove that field and switch to `run_id + repo_id` addressing.
- `internal/store/schema.sql` + `internal/store/queries/run_repos.sql`: contain `execution_run_id` and related queries/indexes that must be removed.

Implementation checklist:

- Remove the “execution run” concept (`run_repos.execution_run_id`) and stop creating child runs per repo.
- Create jobs directly for `(runs.id, run_repos.repo_id)`; persist `jobs.repo_id` + `jobs.repo_base_ref`.
- When a job completes, update:
  - `run_repos.status` for that repo (derived from repo-scoped jobs)
  - `runs.status` for the run (derived from all repos)

Status derivation note: see `roadmap/v1/statuses.md`.
