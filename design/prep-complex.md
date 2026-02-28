# Prep Complex Mode (Target Design, Not Yet Implemented)

## Status

Complex prep profile shape is defined, but complex orchestration execution is not implemented in the current runtime.

Current prep execution path is simple-mode Codex output validation and persistence.

## Target Definition

Complex mode is for repositories that cannot be prepared by command/env plus runtime hints alone, and require lifecycle orchestration around build/test execution.

Typical triggers:
- sidecar daemon/container lifecycle requirements
- registry auth/trust lifecycle setup
- pre/post execution resource provisioning and cleanup

## Contract Surface

Complex profile uses:
- `runner_mode: complex`
- non-empty `orchestration.pre` and/or `orchestration.post`
- same target result structure as simple mode

Declared orchestration primitive types (schema whitelist):
- `docker_network`
- `docker_network_remove`
- `docker_container`
- `docker_remove`
- `wait_for_log`
- `health_check`

## Required Runtime Guarantees (Future Implementation)

When complex execution is implemented, it must guarantee:
1. `pre` steps run before target command execution.
2. `post` cleanup steps run on both success and failure paths.
3. step-level logging and deterministic status reporting.
4. bounded retries and timeouts.
5. no backward-compat fallback to ad-hoc orchestration formats.

## Router-Guided Loop Policy (Adopted)

Complex recovery flow adopts a router-guided loop:
- router executes after every gate failure, including each failed `re_gate`
- router classification drives prompt strategy and stopping policy
- loop context persists across iterations (history + loop kind)
- current `loop_kind` value is `healing` for all strategies (reserved for future loop families)

Classification outcomes:
- `infra`
- `code`
- `mixed`
- `unknown`

Stopping rule (current decision):
- `mixed` or `unknown` is terminal for the repo attempt (stop mig progression)

Strategy routing policy:
- `error_kind` chooses strategy (prompt/tooling/output contract)
- phase is part of strategy input (pre/post/re gate signal), not a separate loop selector

`infra` strategy contract:
- expected typed artifact: profile candidate (for example `/out/prep-profile-candidate.json`)
- candidate may be promoted to repo default prep profile only after schema validation and successful re-gate

## Router Prompt Packaging (Adopted)

Introduce `router/` folder for router assets:
- phase-aware classification prompts
- strategy templates for `infra` and `code` routing
- schema/contracts for router JSON output and strategy artifacts

Goal:
- keep router behavior versioned, explicit, and shared across all error-kind strategies.

## Separation From Simple Mode

`runtime.docker.mode` is simple runtime hinting, not complex orchestration by itself.

Complex mode starts only when lifecycle orchestration primitives are required.

## Acceptance Criteria For Complex Track

Complex track is complete only when:
- orchestration executor is wired in production path
- declared primitives execute with deterministic semantics
- cleanup is guaranteed on failure paths
- end-to-end tests cover complex success and cleanup-failure scenarios

## Cross References

- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-states.md`
- `docs/schemas/prep_profile.schema.json`
- `roadmap/prep/track-1-minimal-e2e.md`
