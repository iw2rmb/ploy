# Migs E2E Tests

The test layers prove different contracts. Unit and integration tests are the
default validation. Live end-to-end (E2E) tests are required for changes that
affect source hydration, job scheduling, node materialization, container
execution, Hydra mounts, or artifact transfer.

An offline test does not replace a live E2E test. Offline tests validate
parsers, contracts, and scenario construction. Live tests validate the full
CLI, control-plane, node, and container path.

## Hydra suite

The Hydra suite contains these live scenarios:

- `scenario-hydra-mount-enforcement` verifies read-only `/in` mounts and
  writable `/out` mounts.
- `scenario-hydra-out-upload` verifies the `/out` artifact round trip.
- `scenario-in-mixed` verifies file and directory inputs.
- `scenario-bundle-blocked` verifies traversal and symlink rejection.

These scenarios do not validate Build Gate. Each generated spec sets
`build_gate.disabled: true`. The source repository supplies only repository
identity and a stable commit.

Build the CLI and set the live test environment:

```bash
make build
export PLOY_SERVER_URL=https://ploy.example.test
export PLOY_AUTH_TOKEN=...
export PLOY_E2E_REPO_OVERRIDE=namespace/ploy-helloworld.git
export PLOY_E2E_BASE_REF=master
export PLOY_E2E_IMAGE=registry.example.test/ploy/e2e-shell:latest
export GITLAB_TOKEN=...
```

`PLOY_E2E_IMAGE` must name a shell-capable image that the Ploy nodes can pull.
The suite does not use a Docker Hub default. The image is only the execution
environment for the Hydra checks; it does not select a source stack.

`GITLAB_TOKEN` is required when the source repository is not public. The
harness passes the environment-variable name through
`--gitlab-token-env GITLAB_TOKEN`; it does not put the token value in process
arguments.

The harness polls JSON status for automation. It does not start the interactive
follow TUI. `PLOY_E2E_TIMEOUT_SECONDS` controls the run timeout and defaults to
600 seconds.

Run the live suite explicitly:

```bash
PLOY_E2E_CLUSTER=require go test -count=1 -v ./tests/e2e/migs
```

Without `PLOY_E2E_CLUSTER=require`, the live tests skip. The deterministic
offline tests continue to run.

## Stack-aware scenarios

Build Gate and Java migration scenarios use a separate Maven repository. For
example, `scenario-stack-aware-images` uses
`iw2rmb/ploy-orw-java11-maven:main`. Do not use the generic Hydra repository
for a stack-aware scenario.

## Smoke scenario

Run the minimal container and log smoke test with:

```bash
bash tests/e2e/migs/scenario-selftest.sh
```

The selftest disables Build Gate because it validates container execution and
job-log delivery only.
