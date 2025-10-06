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

- `GRID_ENDPOINT` — Points the CLI at the Grid Workflow RPC. Discovery
  configures JetStream routes and the IPFS gateway automatically. Current
  default: `unset` (the CLI uses the in-memory Grid stub).
- `GRID_API_KEY` — Optional bearer token supplied to the Grid helper and
  discovery requests via the `Authorization` header.
- `GRID_ID` — Optional identifier used to scope SDK state directories and
  discovery headers when connecting to multiple Grid installations.
- `GRID_WORKFLOW_SDK_STATE_DIR` — Overrides the Workflow RPC SDK state
  directory. Defaults to `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid`, which
  Ploy now sets up automatically so manifest caches and CA bundles persist
  across CLI runs.
- `PLOY_LANES_DIR` — Explicit path to the lane catalog directory. When unset,
  Ploy searches `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/lanes`, `$HOME/.ploy/lanes`,
  and the adjacent `../ploy-lanes-catalog` checkout (for development). Set this
  before running the CLI to avoid missing lane definitions.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).

## E2E Harness

- `PLOY_E2E_TENANT` — Tenant slug consumed by the Mods E2E harness when running
  `ploy workflow run` against Grid.
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
  settings are discovered dynamically from the configured `GRID_ENDPOINT`.

## gapi

- No environment variables are active for gapi within this codebase; record
  future additions here once the API slices land.

## Related Docs

- [docs/design/overview/README.md](../design/overview/README.md)
- [docs/design/workflow-rpc-alignment/README.md](../design/workflow-rpc-alignment/README.md)
- [docs/design/ipfs-artifacts/README.md](../design/ipfs-artifacts/README.md)
- [docs/design/snapshot-metadata/README.md](../design/snapshot-metadata/README.md)
