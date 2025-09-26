# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid, reconstructs the default mods→build→test DAG, and dispatches stages to the in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]
```
The command boots in-memory JetStream and Grid stubs, claims a ticket (auto-generating one if `--ticket auto`), publishes checkpoints for every stage transition, executes mods/build/test against a temporary workspace, and cleans up before exit. Upcoming roadmap slices will swap the stubs for real JetStream connections and Grid RPC calls.

## Flags
- `--tenant` — Tenant slug used to resolve subject namespaces. Required.
- `--ticket` — JetStream ticket identifier to claim. Defaults to `auto`, which selects or generates the next ticket on the tenant stub.

## Exit Codes
- `0` — success (ticket claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or downstream error).

## Environment
- ``JETSTREAM_URL`` — TODO (real endpoint wired in Grid integration slice).
- ``GRID_ENDPOINT`` — TODO (real Workflow RPC target lands with JetStream wiring).
- ``IPFS_GATEWAY`` — TODO (snapshot/artifact publishing slice).
The current stubs ignore these values; they are documented so workstation environments can surface them once integration resumes.

## Development
- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test`.
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI focused on stateless execution against JetStream/Grid contracts.
