# Prep Implementation Outline

## Goal

Define a pragmatic implementation path for prep with fast end-to-end value first, then progressive hardening.

## Implementation Tracks

## 1. Minimal but E2E Process

Objective:
- Ship the smallest complete flow that can prep a new repo and unblock migration.

Scope:
- Trigger prep on new repo registration.
- Run one non-interactive prep session with default prompt.
- Validate output against prep profile schema.
- Persist profile + prep result.
- Gate next stage on `PrepReady`.

Tactics:
- Support `simple` mode first.
- Record `complex` findings as failure evidence (no orchestration execution yet).
- Add one reproducibility rerun before marking success.
- In simple mode, allow minimal runtime hints (`runtime.docker.mode`) without orchestration.

Deliverables:
- Repo prep state transitions wired to lifecycle.
- Prep run artifact storage (attempt logs + diagnostics).
- Build gate planner reads profile commands/env when present.

Acceptance:
- New repo can go `PrepPending -> PrepRunning -> PrepReady` with persisted profile.
- Failed prep lands in `PrepFailed` with actionable failure codes and logs.

## 2. Simple Schema Hardening

Objective:
- Make simple-mode outputs strict, stable, and easy to validate.

Scope:
- Tighten `docs/schemas/prep_profile.schema.json` invariants for simple flows.
- Add semantic checks beyond JSON shape.

Tactics:
- Enforce target/result consistency:
  - `passed` requires command and null failure code.
  - `failed` requires command and concrete failure code.
- Split simple profiles into:
  - `simple_core` (command/env only)
  - `simple_runtime` (command/env + runtime hints)
- Enforce simple-mode orchestration constraints:
  - `orchestration.pre` and `orchestration.post` must be empty.
- Enforce runtime hint constraints:
  - `runtime.docker.mode` enum (`none|host_socket|tcp`)
  - `runtime.docker.host` required only for `tcp`.
- Enforce stable enums for status and failure taxonomy.
- Add schema versioning policy (`schema_version` required; additive changes only for v1).
- Add CI validation fixture set:
  - valid simple profile(s)
  - invalid profile samples for each invariant

Deliverables:
- Hardened schema and test fixtures.
- Validation utility used by prep orchestrator before persistence.

Acceptance:
- Invalid simple profiles are rejected deterministically with clear reason.
- Backward-compatible v1 profiles continue to validate.

## 3. Complex Schema

Objective:
- Support repos that require orchestration (daemon/service/auth/trust lifecycle) without ad-hoc scripts.

Scope:
- Extend schema to represent complex orchestration declaratively.
- Execute orchestration steps via controlled runner primitives.

Tactics:
- Keep orchestration primitive whitelist only:
  - `docker_network`
  - `docker_network_remove`
  - `docker_container`
  - `docker_remove`
  - `wait_for_log`
  - `health_check`
- Require explicit cleanup in post-steps.
- Require rerun from fresh orchestration lifecycle before success.
- Store capability requirements in profile (`runner_mode=complex`, required envs).

Deliverables:
- Complex profile validation + runner adapter.
- Deterministic orchestration logs and step-level statuses.

Acceptance:
- Complex scenario can pass end-to-end using declarative profile only.
- Cleanup always runs, including failure paths.

## 4. Feedback Loop Hardening

Objective:
- Improve default prompt/tactics from successful prep runs without destabilizing behavior.

Scope:
- Collect reusable findings from successful runs.
- Propose and roll out prompt/tactics updates safely.

Tactics:
- Store `prompt_delta_suggestion` from prep outputs.
- Add adjudication step before global prompt/tactic updates:
  - deduplicate
  - classify as repo-local vs cross-repo
  - require evidence quality threshold
- Roll out updates via canary:
  - apply to subset of new repos
  - monitor failure-code drift and success rate
- Redact secrets from all harvested artifacts.

Deliverables:
- Prompt/tactics candidate queue.
- Review and rollout policy with metrics.
- Regression guardrails (automatic rollback on degradation).

Acceptance:
- Feedback loop improves prep success rate without increasing false-success profiles.
- Prompt/tactics updates are auditable and reversible.

## Suggested Delivery Order

1. Track 1 (E2E minimal)
2. Track 2 (simple hardening)
3. Track 3 (complex orchestration)
4. Track 4 (feedback hardening)

## Cross References

- `design/prep.md`
- `design/prep-states.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-prompt.md`
- `docs/schemas/prep_profile.schema.json`
