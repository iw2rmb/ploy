# Gate Profile Simple Mode (Implemented)

## Definition

Simple mode is the active gate profile execution contract.

A profile is simple when it provides deterministic target command/env outcomes without orchestration steps.

## Required Profile Surface

Simple mode requirements:
- `runner_mode: simple`
- non-empty `stack.language`
- non-empty `stack.tool`
- `targets.active: build|unit|all_tests|unsupported`
- `orchestration.pre: []`
- `orchestration.post: []`
- target results for `build`, `unit`, `all_tests`

Allowed runtime hints:
- `runtime.docker.mode`: `none | host_socket | tcp`
- `runtime.docker.host`: required for `tcp`
- `runtime.docker.api_version`: optional

## Build Gate Mapping (As-Built)

Claim-time phase mapping:
- `pre_gate` writes override to `build_gate.pre.gate_profile`
- `post_gate` and `re_gate` write override to `build_gate.post.gate_profile`
- command/env source for all gate phases is `targets.<targets.active>`
- runtime ignores target `status`/`failure_code` for command/env selection
- runtime does not auto-fallback across targets
- `targets.active=unsupported` is terminal and injects no runnable override
- unsupported contract requires `targets.build.status=failed` and `targets.build.failure_code=infra_support`

Mapped runtime hints:
- host socket mode -> `DOCKER_HOST=unix:///var/run/docker.sock`
- tcp mode -> `DOCKER_HOST=<runtime.docker.host>`
- api version -> `DOCKER_API_VERSION=<value>`

## Stack-Bound Enforcement

Claim-time gate_profile override carries profile stack into `build_gate.<phase>.gate_profile.stack`.

Gate runtime enforces gate_profile stack compatibility. If gate_profile stack mismatches the gate stack context, gate command resolution fails instead of executing stale gate_profile commands.

## Command and Env Precedence

Gate command precedence:
1. explicit `build_gate.<phase>.gate_profile` in submitted spec
2. mapped gate profile override
3. default tool command

Env precedence:
1. base gate env from spec/global injection
2. gate_profile override env (explicit or mapped)

## Recovery Interaction

Simple-mode profile participates in the shared recovery loop:
- `gate -> router -> healing -> re_gate`
- router emits `error_kind`: `infra|code|mixed|unknown`
- server injects `build_gate.healing.selected_error_kind` for heal claims
- `mixed` and `unknown` are terminal

Infra path contract:
- expected candidate artifact: `/out/gate-profile-candidate.json`
- expected schema id: `gate_profile_v1`
- candidate can be promoted only after successful follow-up `re_gate`

## Notes

Simple mode is the only fully wired gate profile mode in runtime execution paths.

## Cross References

- `design/gate-profile.md`
- `design/gate-profile-impl.md`
- `design/gate-profile-states.md`
- `design/gate-profile-complex.md`
- `docs/schemas/gate_profile.schema.json`
- `docs/build-gate/README.md`
