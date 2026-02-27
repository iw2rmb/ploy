# Prep Stage for Repo Build Readiness

## Goal

Add a mandatory `prep` stage for every repository newly added to Ploy.

The stage discovers reproducible build settings with strict priority:
1. `build`
2. `unit tests`
3. `all tests`

If prep succeeds, the repository proceeds to normal migration flow with a persisted repo-specific build profile.

## Problem

Repositories in scope are heterogeneous:
- different tools and runtime versions
- different test partitioning conventions
- different container/runtime assumptions
- different internal registry and certificate requirements

Default build gate commands fail for many repos, blocking migration even when a valid build path exists.

## Scope

In scope:
- non-interactive discovery of executable build/test setup
- persistence of repo-specific instructions
- global tactics feedback loop from successful discoveries

Out of scope:
- changing repository source code during prep
- long-running human-in-the-loop debugging in prep
- replacing build gate itself

## High-Level Design

### Components

1. Prep Orchestrator
- Runs before first migration for a repo.
- Executes a bounded non-interactive prep session.
- Produces normalized result artifacts.

2. Prep Agent Session (Codex non-interactive)
- Receives a fixed prompt and tool budget.
- Attempts tactics in priority order.
- Returns structured output with evidence.

3. Repo Build Profile Store
- Stores per-repo resolved commands, env, and orchestration requirements.
- Used by build gate and migration pipeline.

4. Global Tactics Catalog
- Shared ordered tactics list.
- Updated from successful prep outcomes after validation.

### Execution Flow

1. Repo is marked `PrepPending` when first registered.
2. Orchestrator starts prep session with default prompt + latest tactics catalog.
3. Session attempts to satisfy targets in order: `build`, `unit`, `all`.
4. Session emits:
- resolved configuration
- command log with exit statuses
- failure taxonomy for unresolved targets
5. Orchestrator validates reproducibility with one clean rerun.
6. On success:
- persist repo profile
- mark repo `PrepReady`
- enqueue normal next step
7. On failure:
- persist evidence
- mark repo `PrepFailed`
- require manual action or retry policy

## Data Contract

Prep output must be machine-readable and stable.

Canonical schema:
- `docs/schemas/prep_profile.schema.json`

Minimum fields:
- `repo_id`
- `targets`:
  - `build`: `passed|failed|not_attempted`
  - `unit`: `passed|failed|not_attempted`
  - `all_tests`: `passed|failed|not_attempted`
- `runner_mode`: `simple|complex`
- `targets.<target>.command` and `targets.<target>.env`
- `runtime`: optional minimal runtime hints for simple mode (e.g. `runtime.docker.mode`)
- `orchestration`: required object; empty arrays for simple mode, populated lifecycle steps for complex mode
- `evidence`: references to logs and key diagnostics
- `tactics_used`: ordered identifiers
- `notes`: short operational caveats

## Readiness Semantics

A repo is considered prep-ready when:
- `build` passes, and
- `unit` passes if unit target is discoverable, and
- resolved configuration reruns successfully once in clean environment

`all_tests` is best-effort in prep. Failure here must not block migration when `build` + `unit` are stable, but must be recorded.

## Failure Taxonomy

Store standardized failure codes to support automation and reporting:
- `tool_not_detected`
- `runtime_version_mismatch`
- `docker_api_mismatch`
- `registry_auth_failed`
- `registry_ca_trust_failed`
- `external_service_unreachable`
- `command_not_found`
- `timeout`
- `unknown`

## Integration Points

1. Control Plane
- Add repo prep state and prep result metadata.

2. Build Gate Planner
- Prefer repo profile over generic default commands when present.

3. Node Agent
- Execute orchestration pre-hooks and post-hooks for complex profiles.

4. Docs and Ops
- Expose prep profile and last prep evidence in run diagnostics.

## Security and Guardrails

- Redact secrets from stored logs and prompts.
- Enforce hard timeout and max attempts.
- Restrict allowed orchestration operations to approved templates.
- Keep prep non-interactive by default.

## Rollout

1. Phase 1: Passive mode
- Run prep, store results, do not gate migration.

2. Phase 2: Soft gate
- Require successful prep for new repos, with manual override.

3. Phase 3: Strict gate
- Prep mandatory for all newly onboarded repos.

## Cross References

- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-prompt.md`
- `design/prep-states.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
