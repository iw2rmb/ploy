# Prep Complex Mode (Future Track)

## Status

Complex profile shape exists in schema/contracts, but there is no active runtime executor for orchestration primitives.

Complex mode is currently a design target layered on top of the existing gate/router/healing loop.

## Target Definition

Complex mode is for repos that cannot be represented by command/env and runtime hints alone, and need explicit lifecycle orchestration.

Typical triggers:
- sidecar daemon/container lifecycle
- registry auth or trust bootstrapping lifecycle
- pre/post resource provisioning and cleanup

## Contract Surface

Complex profile uses:
- `runner_mode: complex`
- required `stack` identity
- non-empty `orchestration.pre` and/or `orchestration.post`
- same target result structure as simple mode

Declared orchestration primitive whitelist:
- `docker_network`
- `docker_network_remove`
- `docker_container`
- `docker_remove`
- `wait_for_log`
- `health_check`

## Required Runtime Guarantees (When Implemented)

1. `pre` steps run before target command execution.
2. `post` cleanup runs on success and failure paths.
3. step-level logging and deterministic status reporting.
4. bounded retries and explicit timeouts.
5. no fallback to ad-hoc orchestration formats.

## Shared Recovery Policy (Already Active)

Even without complex execution, recovery policy is already fixed and shared:
- router executes after every gate failure, including failed `re_gate`
- router emits `error_kind` in `infra|code|mixed|unknown`
- `mixed` and `unknown` are terminal for the repo attempt
- `infra` and `code` route to `build_gate.healing.by_error_kind.<error_kind>`
- server injects `build_gate.healing.selected_error_kind` on heal claims

Infra candidate contract:
- expected artifact path: `/out/prep-profile-candidate.json`
- expected schema id: `prep_profile_v1`
- candidate must pass schema and stack checks
- promotion requires successful follow-up `re_gate`

## Relationship to Simple Mode

`runtime.docker.mode` is a simple-mode runtime hint, not complex orchestration.

Complex mode starts only when orchestration primitives must be executed with deterministic lifecycle semantics.

## Acceptance Criteria for Complex Track

Complex track is complete only when:
- orchestration executor is wired in production path
- primitive semantics are deterministic
- cleanup is guaranteed on failures
- e2e coverage includes success and cleanup-failure cases
- promotion/re-gate behavior remains compatible with the shared recovery contract

## Cross References

- `design/prep.md`
- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-states.md`
- `docs/schemas/prep_profile.schema.json`
