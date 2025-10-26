# Environment Variables

This reference tracks the environment variables that the workstation CLI
inspects today and notes the current local values. Update this file whenever a
new variable is introduced, defaults change, or components adopt additional
configuration.

## Dependencies

- [cmd/ploy/dependencies.go](../../cmd/ploy/dependencies.go) — runtime factories
  resolving Grid, JetStream, and IPFS endpoints.
- [cmd/ploy/feature_flags.go](../../cmd/ploy/feature_flags.go) — feature flag
  inspection for the Aster integration.

## CLI

- `PLOY_GRID_ID` — Optional legacy Grid identifier. Provide only when running
  against the legacy Grid stack; the SSH-descriptor workflow does not require
  it.
- `GRID_BEACON_API_KEY` / `GRID_BEACON_URL` — Legacy beacon credentials used
  when talking to the Grid control plane. Unset for the SSH-only workflow.
- `GRID_CLIENT_STATE_DIR` / `GRID_WORKFLOW_SDK_STATE_DIR` — Legacy overrides for
  the Grid client's state directory.
- `PLOY_RUNTIME_ADAPTER` — Optional runtime adapter selector. Defaults to
  `local-step`. Other adapters (`grid`, `k8s`, `nomad`) plug in here; the CLI
  fails fast when an unknown adapter name is provided.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).
- `PLOY_IPFS_CLUSTER_API` — Base URL for the IPFS Cluster REST API used by the
  step runtime and CLI artifact commands. Required when artifact publishing is
  enabled.
- `PLOY_IPFS_CLUSTER_TOKEN` — Optional bearer token passed to the cluster when
  authenticating artifact requests.
- `PLOY_IPFS_CLUSTER_USERNAME` / `PLOY_IPFS_CLUSTER_PASSWORD` — Optional
  basic-auth credentials used when a bearer token is not available. Username
  and password must be provided together.
- `PLOY_IPFS_CLUSTER_REPL_MIN` — Optional override for the minimum replication
  factor applied to artifact pins. Defaults to the cluster-defined value when
  unset or zero.
- `PLOY_IPFS_CLUSTER_REPL_MAX` — Optional override for the maximum replication
  factor applied to artifact pins. Defaults to the cluster-defined value when
  unset or zero.
- `PLOY_ETCD_USERNAME` / `PLOY_ETCD_PASSWORD` — Optional etcd basic-auth credentials applied when
  ployd connects to the local etcd instance.
- `PLOY_ETCD_TLS_CA` — Path to a PEM bundle used to trust etcd server certificates. Optional.
- `PLOY_ETCD_TLS_CERT` / `PLOY_ETCD_TLS_KEY` — Optional client certificate pair presented to etcd
  when mutual TLS is required. Both values must be provided together.
- `PLOY_ETCD_TLS_SKIP_VERIFY` — When set to `true`, disables server certificate verification. Use
  only for local development.
- `PLOYD_CONFIG_PATH` — When set during bootstrap, overrides the generated ployd configuration file
  location (default `/etc/ploy/ployd.yaml`).
- `PLOYD_HTTP_LISTEN` — Optional address override for the ployd HTTP API listener when bootstrap
  generates the initial configuration (default `0.0.0.0:8443`).
- `PLOYD_METRICS_LISTEN` — Optional override for the ployd Prometheus metrics listener (defaults to
  `:9100`).
- `PLOY_SSH_USER` — SSH username applied when establishing control-plane tunnels (default `ploy`).
- `PLOY_SSH_IDENTITY` — Path to the SSH private key used for tunnel authentication (default `~/.ssh/id_rsa`).
- `PLOY_SSH_SOCKET_DIR` — Override for the directory holding SSH control sockets (default `~/.ploy/tunnels`).
- `PLOY_CACHE_HOME` — Optional base directory for CLI cache artifacts such as tunnel node assignments.
- `PLOY_ARTIFACT_ROOT` — Optional override for the local artifact cache used by the step workspace hydrator and filesystem artifact publisher. Defaults to `$XDG_CACHE_HOME/ploy/artifacts` (or the OS cache dir fallback) when unset.
- `PLOY_SCHEDULER_MODE` — Selects the control-plane backend (`grid` or `etcd`). Defaults to `grid`
  until the CLI flips to the new scheduler by default.

## E2E Harness

- `PLOY_E2E_TENANT` — Tenant slug consumed by the Mods E2E harness when running
  `ploy mod run` against Grid.
- `PLOY_E2E_TICKET_PREFIX` — Optional ticket ID prefix for Mods E2E runs
  (default `e2e`).
- `PLOY_E2E_REPO_OVERRIDE` — Optional Git repository override used by the Mods
  E2E scenarios in place of the default Java sample repo.
- `PLOY_E2E_GITLAB_TOKEN` — Optional GitLab PAT so the E2E harness can clean up
  branches after creating merge requests.
- `PLOY_E2E_LIVE_SCENARIOS` — Optional comma-separated scenario IDs that the
  live Grid smoke test should execute (defaults to `simple-openrewrite`).

## Grid (legacy service)

- No additional environment variables are managed inside this repository
  slice; the legacy Grid client discovers settings via `sdk/gridclient/go`
  using `PLOY_GRID_ID` plus the optional beacon credentials above.

## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Control Plane

- `PLOY_GITLAB_SIGNER_AES_KEY` — Required base64-encoded AES key used by the signer
  to encrypt GitLab API keys before persisting them in etcd. The decoded key must be
  16, 24, or 32 bytes to satisfy AES-GCM requirements.
- `PLOY_CONTROL_PLANE_URL` — Base URL for control-plane HTTP APIs (`ploy config gitlab`,
  worker onboarding via `ploy cluster add --cluster-id`, etc.). Required now that cluster descriptors only record SSH
  metadata and no longer carry beacon/control-plane URLs.
- `PLOY_GITLAB_SIGNER_DEFAULT_TTL` — Optional duration (e.g., `15m`) applied when
  callers omit a TTL while requesting short-lived GitLab tokens.
- `PLOY_GITLAB_SIGNER_MAX_TTL` — Optional duration that caps the maximum issued TTL.
  Requests above this threshold are rejected. Defaults to `12h` when unset.
- `PLOY_GITLAB_API_BASE_URL` — Base URL for GitLab API requests when revoking
  stale runner tokens during credential rotations.
- `PLOY_GITLAB_ADMIN_TOKEN` — Admin or automation token presented to GitLab when
  calling the revocation API. Required for the rotation revocation workflow to
  disable stale tokens across nodes.

## Related Docs

- [docs/design/overview/README.md](../design/overview/README.md)
- [docs/design/workflow-rpc-alignment/README.md](../design/workflow-rpc-alignment/README.md)
- [docs/design/ipfs-artifacts/README.md](../design/ipfs-artifacts/README.md)
- [docs/design/snapshot-metadata/README.md](../design/snapshot-metadata/README.md)
