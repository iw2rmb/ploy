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
- `GRID_WORKFLOW_SDK_STATE_DIR` — Overrides the Workflow RPC SDK state
  directory. Defaults to `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid`, which
  Ploy now sets up automatically so manifest caches and CA bundles persist
  across CLI runs.
- `PLOY_ASTER_ENABLE` — Opt-in switch for the experimental Aster bundle
  integration. Current default: `unset` (Aster toggles stay disabled).

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
