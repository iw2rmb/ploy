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
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).

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
