# Roadmap v1 — Scope

## Deltas vs HEAD

- Change: treat “mods” as long-lived projects (name/specs/repo set) instead of “run submission”.
  - Where: server routes in `internal/server/handlers/register.go` and new handlers for `/v1/mods/*` project CRUD.
  - Compatibility impact: breaking API; no backward compatibility required.
- Change: remove “execution run per repo” (`run_repos.execution_run_id`) and model execution as `runs` → `run_repos`.
  - Where: `internal/store/schema.sql`, `internal/store/queries/run_repos.sql`, `internal/server/handlers/runs_batch_scheduler.go`, `internal/server/handlers/mods_ticket.go`.
  - Compatibility impact: breaking DB + scheduling semantics; no backward compatibility required.
- Change: make run artifacts repo-scoped under `/v1/runs/{run_id}/repos/{repo_id}/...` and derive repo attribution from `job_id → jobs.repo_id`.
  - Where: `internal/server/handlers/events.go` (SSE today is run-scoped), `internal/server/handlers/diffs.go` (diffs today are `/v1/mods/{run_id}/diffs`), plus new repo-scoped handlers.
  - Compatibility impact: breaking for clients; no backward compatibility required.

## Goal

Support “code modification projects” where:

- A **mod** is a long-lived project with a unique name.
- A mod has **spec variants** (Mods YAML/JSON) to iterate on approach (ORW recipes, LLM model/prompt, etc.).
- A mod has a managed **repo set** (identified by `repo_url`) that changes over time.
- A **run** executes a chosen spec variant over:
  - one repo,
  - a selected subset of repos,
  - or the mod’s repos whose last run state is `Failed`.

Run entrypoints:

- `ploy run --spec ... --repo ...` creates a run and immediately starts execution (single-repo). It also creates a mod project as a side-effect; the created mod has `name == id`.
- `ploy mod run <mod> ...` creates a run for a mod project and immediately starts execution over the mod’s repo set.

## Terms (no new nouns)

- **Mod**: project container (unique name).
- **Spec**: a Mods spec variant (stored as JSONB; authored as YAML/JSON).
- **Repo**: a repo participating in a mod (repo_url + refs).
- **Run**: an execution attempt; produces run-level and per-repo status, artifacts, logs, diffs.

Repo URL note (v0 reference):

- Current repo URL validation accepts only `https://`, `ssh://`, and `file://` URLs (see `internal/domain/types/vcs.go`, `RepoURL.Validate`).
- v0 `ploy mod run pull` normalization examples include scp-style `git@host:owner/repo` (see `cmd/ploy/mod_run_pull.go`, `normalizeRepoURLForCLI`), but v0 server-side repo endpoints validate `RepoURL` (see `internal/server/handlers/repos.go`).

## Non-goals (v1)

- Cross-mod spec sharing.
- Automatic repo discovery from orgs/monorepos.
- Scoring frameworks beyond storing basic metrics + optional human score.
- Backward compatibility layers or migrations for legacy “runs-only” workflows.

## Key behaviors

- **Immutability**: a run links to the exact spec variant used.
- **Stable grouping**: grouping is by `mods.name` (unique) and `runs.mod_id` (no `runs.name`).
- **Archiving**: archived mods cannot be executed.
- **Repo refs over time**:
  - `mod_repos` rows are mutable (e.g., CSV import can rewrite refs).
  - `run_repos.repo_base_ref` and `run_repos.repo_target_ref` snapshot refs used for that repo in that run.
- **Repo selection**:
  - `--repo ...` → explicit repos
  - `--failed` → repos whose most recent terminal per-repo status is `Failed`
  - default → all repos in the mod repo set
- **Immediate start**: both `ploy run` and `ploy mod run` start pending work right away.

## Execution model shift (required)

Codebase must switch from “root-run → per-repo execution runs” to “run → run_repos”.

## `/v1/mods/*` route collisions (v0 reference)

Current server routes under `/v1/mods/*` are run-scoped (see `internal/server/handlers/register.go`) and collide with v1’s “mods are projects” direction.

- `POST /v1/mods` (run submission) must move to `POST /v1/runs`.
- Run-scoped routes under `/v1/mods/{run_id}/*` must move under `/v1/runs/{run_id}/*`:
  - examples: `GET /v1/mods/{run_id}/diffs`, `GET /v1/mods/{run_id}/graph`, `POST /v1/mods/{run_id}/cancel`, `POST /v1/mods/{run_id}/resume`.

Current codebase behavior (to remove):

- `runs` row acts as a batch root and stores repo_url/refs/spec.
- Each `run_repos` row may point to a separate execution run via `run_repos.execution_run_id` (this linkage is removed in v1).
- Jobs, logs, diffs, and events are attached to the execution run ID, not the `(run_id, repo_id)` pair.

v1 behavior (to implement):

- One `runs` row represents the run (mod + spec) and holds `runs.mod_id` + `runs.spec_id`.
- Per-repo execution state is represented by `run_repos` rows (scoped to `runs.id`).
- Jobs become repo-scoped by adding `jobs.repo_id` and `jobs.repo_base_ref` (copied from `run_repos`).
- Logs/diffs/events remain addressed by `run_id`, but repo attribution comes from `job_id → jobs.repo_id`.
- Base commit SHA recording moves from `runs.commit_sha` to `run_repos.commit_sha`.

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

Status derivation note:

- A `run_repos` row is `Running` while it has any jobs with `jobs.status IN ('pending','running')` for `(run_id, repo_id)`.
