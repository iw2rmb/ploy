# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid and orchestrates follow-up jobs. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy workflow run --tenant <tenant> --ticket <ticket-id>
```
The command boots an in-memory JetStream stub, claims the requested ticket, and publishes an initial `claimed` checkpoint. Upcoming roadmap slices will wire the CLI to real JetStream, resolve DAGs, and publish job specs back to Grid.

## Flags
- `--tenant` — Tenant slug used to resolve subject namespaces. Required until automatic ticket discovery lands in `02-workflow-runner-cli`.
- `--ticket` — JetStream ticket identifier to claim. Required until ticket auto-discovery lands in a later slice.

## Exit Codes
- `0` — success (ticket claimed and checkpoint published locally).
- `1` — error (missing flags, unsupported subcommand, or downstream failure).

## Development
- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test`.
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI focused on stateless execution.
