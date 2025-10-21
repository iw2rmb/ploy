# Environment Variables

This reference tracks the environment variables that the workstation CLI
(gridctl) inspects today and notes the current local values. Update this file
whenever a new variable is introduced, defaults change, or components adopt
additional configuration.

## Dependencies

- [cmd/ploy/dependencies.go](../../cmd/ploy/dependencies.go) — runtime factories
  resolving Grid, JetStream, and IPFS endpoints.
- [cmd/ploy/feature_flags.go](../../cmd/ploy/feature_flags.go) — feature flag
  inspection for the Aster integration.

## gridctl (CLI)

- `PLOY_GRID_ID` — Required grid identifier used to scope client state on disk and
  construct the discovery/beacon requests. The CLI fails fast when unset.
- `GRID_BEACON_API_KEY` — Required beacon-scoped API key presented to gridbeacon
  when bootstrapping discovery, trust material, and workflow credentials. For
  pre-existing grids, run `gridctl grid client backfill --grid-id <grid>` after
  adopting the new client so beacon publishes the `manifestHost` and CA bundle
  expected by the SDK.
- `GRID_BEACON_URL` — Optional override for the gridbeacon base URL.
  Defaults to the production beacon (`https://beacon.getgrid.dev`).
- `GRID_CLIENT_STATE_DIR` — Optional override for the grid client state
  directory. Defaults to `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid/<grid-id>`
  so discovery caches, manifests, and trust bundles persist per grid.
- `GRID_WORKFLOW_SDK_STATE_DIR` — Legacy override retained for compatibility.
  When set it controls the workflow SDK cache path and is reused as the grid
  client state directory.
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
- `PLOY_ETCD_ENDPOINTS` — Comma-separated etcd endpoints used by the control-plane scheduler
  (e.g., `https://127.0.0.1:2379`). Required when the new scheduler mode is enabled and for
  storing GitLab credentials via `ploy config gitlab`.
- `PLOY_ETCD_USERNAME` / `PLOY_ETCD_PASSWORD` — Optional etcd basic-auth credentials applied when
  connecting to endpoints listed above.
- `PLOY_ETCD_TLS_CA` — Path to a PEM bundle used to trust etcd server certificates. Optional.
- `PLOY_ETCD_TLS_CERT` / `PLOY_ETCD_TLS_KEY` — Optional client certificate pair presented to etcd
  when mutual TLS is required. Both values must be provided together.
- `PLOY_ETCD_TLS_SKIP_VERIFY` — When set to `true`, disables server certificate verification. Use
  only for local development.
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

## Grid (service)

- No environment variables are managed inside this repository slice; Grid
  settings are discovered dynamically via `sdk/gridclient/go` using the inputs
  above (grid ID + beacon API key).

## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Related Docs

- [docs/design/overview/README.md](../design/overview/README.md)
- [docs/design/workflow-rpc-alignment/README.md](../design/workflow-rpc-alignment/README.md)
- [docs/design/ipfs-artifacts/README.md](../design/ipfs-artifacts/README.md)
- [docs/design/snapshot-metadata/README.md](../design/snapshot-metadata/README.md)
