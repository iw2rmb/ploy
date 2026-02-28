# Prep Simple Mode (Implemented)

## Definition

Simple mode is the currently integrated prep path.

A repo is handled as simple when prep can provide deterministic command/env outputs per target without lifecycle orchestration execution.

Simple mode includes Codex integration:
- prep attempt is executed by the non-interactive Codex runner
- output must validate against prep profile schema v1

## Allowed Profile Surface

Simple mode profile requirements:
- `runner_mode: simple`
- `orchestration.pre: []`
- `orchestration.post: []`
- target results for `build`, `unit`, `all_tests`

Allowed runtime hints:
- `runtime.docker.mode`: `none | host_socket | tcp`
- `runtime.docker.host`: required only when mode is `tcp`
- `runtime.docker.api_version`: optional

## Build Gate Mapping (Implemented)

Simple profile targets are mapped to gate phases at claim time:
- `pre_gate` <- `targets.build`
- `post_gate` <- `targets.unit`
- `re_gate` <- `targets.unit`

Mapping applies only when mapped target has:
- `status: passed`
- non-empty `command`

Mapped runtime hints are translated to gate env:
- host socket mode -> `DOCKER_HOST=unix:///var/run/docker.sock`
- tcp mode -> `DOCKER_HOST=<runtime.docker.host>`
- `api_version` -> `DOCKER_API_VERSION=<value>`

## Command/Env Precedence

Gate command precedence (highest wins):
1. `build_gate.<phase>.prep` from submitted spec
2. mapped simple prep profile override
3. default tool command

Env precedence:
1. base gate env from spec/global injection
2. prep override env (explicit or mapped)

## Validation and Success Criteria

For a prep attempt to persist success:
- runner exits successfully
- output JSON parses
- output passes `docs/schemas/prep_profile.schema.json`
- profile and artifacts persist transactionally

Then repo transitions to `PrepReady`.

## Operational Notes

Simple mode is the only end-to-end execution mode currently wired into scheduling and build-gate override behavior.

Complex orchestration remains future scope.

## Recovery Interaction (As-Built)

Gate-recovery behavior for simple-profile runs:
- one loop path is used (`agent -> re_gate`) with `loop_kind=healing`
- router runs on every gate failure, including failed `re_gate`
- router receives gate phase as signal (`pre_gate|post_gate|re_gate`)
- strategy is selected by router `error_kind` (`infra|code|mixed|unknown`)
- healing actions are configured in `build_gate.healing.by_error_kind.<error_kind>`
- server injects `build_gate.healing.selected_error_kind` when claiming heal jobs
- if router classification is `mixed` or `unknown`, mig progression stops for that repo attempt
- `infra` strategy can require artifact `path=/out/prep-profile-candidate.json`, `schema=prep_profile_v1`; promotion to repo default remains gated by validation and successful re-gate

## Cross References

- `design/prep-impl.md`
- `design/prep.md`
- `design/prep-complex.md`
- `docs/schemas/prep_profile.schema.json`
- `docs/build-gate/README.md`
