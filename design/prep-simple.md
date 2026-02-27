# Prep Implementation: Simple Scenarios

## Definition

A repo is `simple` when prep can resolve build/test execution using:
- deterministic commands
- static env vars
- minimal runtime hints only (no lifecycle orchestration)
- no extra service lifecycle management

Typical examples:
- pure unit test repos
- repos where `./gradlew test --tests ...` is sufficient
- repos with only runtime version pinning requirements

## Inputs

- workspace path
- default prep prompt
- tactics catalog entries tagged `simple`
- Codex non-interactive prep runner
- baseline runner capabilities (Docker optional, but not orchestrated)

## Simple Split

Simple mode has two sub-levels:
- `simple_core`: command + env only.
- `simple_runtime`: command + env plus minimal runtime hints.

Allowed simple runtime hints:
- `runtime.docker.mode`: `none | host_socket | tcp`
- `runtime.docker.host`: required only for `tcp`
- `runtime.docker.api_version`: optional

Not allowed in simple mode:
- non-empty `orchestration.pre` or `orchestration.post`
- sidecar lifecycle primitives

## Algorithm

1. Detect build stack/tooling.
2. Generate candidate commands per target:
- `build`
- `unit`
- `all_tests`
3. Try candidates in ordered tactic list.
4. Capture command exit code and compact diagnostic.
5. Stop at first successful command per target.
6. Perform one clean rerun of resolved targets.
7. Persist profile if rerun is stable.

## Command Strategy

Command generation is deterministic and stack-specific.

Examples:
- Gradle:
  - `./gradlew --no-daemon build`
  - `./gradlew --no-daemon test --tests '...unit...'`
  - `./gradlew --no-daemon test`
- Maven:
  - `mvn -B -DskipTests=false clean install`
  - `mvn -B -Dtest='*UnitTest' test`
- Go:
  - `go test ./...`
  - package-filtered unit subsets when repository conventions exist

## Profile Shape (Simple)

```yaml
version: 1
runner_mode: simple
runtime:
  docker:
    mode: host_socket
targets:
  build:
    command: "./gradlew --no-daemon build -x test"
    env: {}
    passed: true
  unit:
    command: "./gradlew --no-daemon test --tests 'com.acme.unit.*'"
    env: {}
    passed: true
  all_tests:
    command: "./gradlew --no-daemon test"
    env: {}
    passed: false
failure_codes:
  all_tests: external_service_unreachable
tactics_used:
  - gradle_unit_filter_by_package
evidence:
  logs:
    - artifacts/prep/build.log
    - artifacts/prep/unit.log
orchestration:
  pre: []
  post: []
```

## Storage and Reuse

Persist profile under repo-scoped prep metadata.

Build gate planner behavior:
- if simple profile exists, use profile commands directly
- fall back to generic gate command only when profile target is absent

## Validation Rules

- command must exit `0` on first pass and clean rerun
- profile is rejected if command depends on transient manual setup
- unresolved targets must include failure code and evidence

## Operational Limits

- max command attempts per target: configurable, default `6`
- per-command timeout: configurable, default `20m`
- total prep timeout: configurable, default `60m`

## Cross References

- `design/prep.md`
- `design/prep-complex.md`
- `design/prep-prompt.md`
- `design/prep-states.md`
- `docs/schemas/prep_profile.schema.json`
