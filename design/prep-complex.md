# Prep Implementation: Complex Scenarios

## Definition

A repo is `complex` when successful execution requires orchestration beyond env+command:
- service containers or sidecars
- registry auth and CA trust setup
- compatibility fallbacks across runtime boundaries

Note:
- docker mode selection alone (`runtime.docker.mode`) belongs to simple-runtime mode.
- complex mode starts when lifecycle orchestration is required.

## Example Signals

- tests use Testcontainers and fail with Docker API negotiation errors
- private registry pulls fail due to auth
- public registry pulls fail due to missing CA trust in daemon context
- command succeeds only with auxiliary daemon/service lifecycle

## Tactic Ladder

Tactics execute in strict order and stop on first stable success.

1. Detect and classify Docker handshake failures
- parse logs for API mismatch (`client too old` / min API)

2. Compatibility env attempt
- try explicit API version env overrides
- validate if negotiation actually changes

3. Old-daemon fallback (sidecar dind)
- start controlled daemon with lower API floor
- target test runner to sidecar daemon

4. Registry and trust hardening
- configure auth for private registry paths
- inject CA bundle into daemon context when required

5. Ryuk and image-prefix adaptations
- disable Ryuk only when required by environment constraints
- override image prefix when private mirror policy blocks pulls

6. Full target rerun and cleanup
- run build/unit/all in resolved mode
- enforce cleanup of temporary network/containers

## Profile Shape (Complex)

```yaml
version: 1
runner_mode: complex
targets:
  build:
    command: "./gradlew --no-daemon build"
    passed: true
  unit:
    command: "./gradlew --no-daemon test --tests 'ru.example.unit.*'"
    passed: true
  all_tests:
    command: "./gradlew --no-daemon test"
    passed: true
env:
  DOCKER_HOST: "tcp://prep-dind:2375"
  TESTCONTAINERS_RYUK_DISABLED: "true"
  TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX: ""
orchestration:
  pre:
    - id: create_network
      type: docker_network
      args: {name: prep-dind-net}
    - id: start_dind
      type: docker_container
      args:
        name: prep-dind
        image: docker:24-dind
        privileged: true
        network: prep-dind-net
        mounts:
          - "/path/ca-certs.pem:/usr/local/share/ca-certificates/corp-root-ca.crt:ro"
  post:
    - id: stop_dind
      type: docker_remove
      args: {name: prep-dind, force: true}
    - id: rm_network
      type: docker_network_remove
      args: {name: prep-dind-net}
tactics_used:
  - docker_socket_mount
  - docker_api_mismatch_detection
  - dind_fallback
  - ca_injection
  - ryuk_disable
failure_codes: {}
```

## Orchestration Contract

Orchestration steps must be declarative and whitelisted.

Allowed primitive types:
- `docker_network`
- `docker_network_remove`
- `docker_container` (bounded options)
- `docker_remove`
- `wait_for_log`
- `health_check`

No arbitrary shell in persistent profiles.

## Validation Rules

- all declared pre-hooks must complete before command execution
- all declared post-hooks must execute even on failure
- profile must include explicit cleanup steps
- success requires clean rerun from fresh sidecar lifecycle

## Observability

Capture additional evidence for complex mode:
- daemon version/API and min API
- registry pull failures by image and host
- CA/auth related diagnostics
- orchestration step timings and exit statuses

## Failure Taxonomy Extensions

- `docker_api_mismatch`
- `docker_daemon_unreachable`
- `registry_auth_failed`
- `registry_ca_trust_failed`
- `sidecar_start_failed`
- `orchestration_cleanup_failed`

## Cross References

- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-prompt.md`
- `design/prep-states.md`
- `docs/schemas/prep_profile.schema.json`
