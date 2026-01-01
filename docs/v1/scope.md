# Roadmap v1 — Scope

## Goal

Support “code modification projects” where:

- A **mod** is a long-lived project with a unique name.
- A mod has **spec variants** (Mods YAML/JSON) to iterate on approach (ORW recipes, LLM model/prompt, etc.).
- A mod has a managed **repo set** (identified by `repo_url`) that changes over time.
- A **run** executes a chosen spec variant over:
  - one repo,
  - a selected subset of repos,
  - or the mod’s repos whose last run state is `failed`.

Run entrypoints:

- `ploy run --spec ... --repo ...` creates a run and immediately starts execution (single-repo). It also creates a mod project as a side-effect; the created mod has `name == id`.
- `ploy mod run <mod> ...` creates a run for a mod project and immediately starts execution over the mod’s repo set.

## Terms (no new nouns)

- **Mod**: project container (unique name).
- **Spec**: a Mods spec variant (stored as JSONB; authored as YAML/JSON).
- **Repo**: a repo participating in a mod (repo_url + refs).
- **Run**: an execution attempt; produces run-level and per-repo status, artifacts, logs, diffs.

## Non-goals (v1)

- Cross-mod spec sharing.
- Automatic repo discovery from orgs/monorepos.
- Scoring frameworks beyond storing basic metrics + optional human score.
- Backward compatibility layers or migrations for legacy “runs-only” workflows.

## Key behaviors

- **Immutability**: a run links to the exact spec variant used.
- **Stable grouping**: grouping is by `mods.name` (unique) and `runs.mod_id` (no `runs.name`).
- **Archiving**: archived mods cannot be executed.
- **Repo selection**:
  - `--repo ...` → explicit repos
  - `--failed` → repos whose most recent terminal per-repo status is `failed`
  - default → all repos in the mod repo set
- **Immediate start**: both `ploy run` and `ploy mod run` start pending work right away.

## Minimal blast radius

- New storage for mod/spec/repo management.
- Link existing `runs` / `run_repos` to mod/spec/repo rows for history and filtering.
