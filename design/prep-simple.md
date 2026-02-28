# Prep Simple Mode (Implemented)

## Definition

Simple mode is the active prep profile execution contract.

A profile is simple when it provides deterministic target command/env outcomes without orchestration steps.

## Required Profile Surface

Simple mode requirements:
- `runner_mode: simple`
- non-empty `stack.language`
- non-empty `stack.tool`
- `orchestration.pre: []`
- `orchestration.post: []`
- target results for `build`, `unit`, `all_tests`

Allowed runtime hints:
- `runtime.docker.mode`: `none | host_socket | tcp`
- `runtime.docker.host`: required for `tcp`
- `runtime.docker.api_version`: optional

## Build Gate Mapping (As-Built)

Claim-time phase mapping:
- `pre_gate` <- `targets.build`
- `post_gate` <- `targets.unit`
- `re_gate` <- `targets.unit`

Mapping applies only when mapped target has:
- `status: passed`
- non-empty `command`

Mapped runtime hints:
- host socket mode -> `DOCKER_HOST=unix:///var/run/docker.sock`
- tcp mode -> `DOCKER_HOST=<runtime.docker.host>`
- api version -> `DOCKER_API_VERSION=<value>`

## Stack-Bound Enforcement

Claim-time prep override carries profile stack into `build_gate.<phase>.prep.stack`.

Gate runtime enforces prep stack compatibility. If prep stack mismatches the gate stack context, gate command resolution fails instead of executing stale prep commands.

## Command and Env Precedence

Gate command precedence:
1. explicit `build_gate.<phase>.prep` in submitted spec
2. mapped prep profile override
3. default tool command

Env precedence:
1. base gate env from spec/global injection
2. prep override env (explicit or mapped)

## Recovery Interaction

Simple-mode profile participates in the shared recovery loop:
- `gate -> router -> healing -> re_gate`
- router emits `error_kind`: `infra|code|mixed|unknown`
- server injects `build_gate.healing.selected_error_kind` for heal claims
- `mixed` and `unknown` are terminal

Infra path contract:
- expected candidate artifact: `/out/prep-profile-candidate.json`
- expected schema id: `prep_profile_v1`
- candidate can be promoted only after successful follow-up `re_gate`

## Notes

Simple mode is the only fully wired prep profile mode in runtime execution paths.

## Cross References

- `design/prep.md`
- `design/prep-impl.md`
- `design/prep-states.md`
- `design/prep-complex.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
