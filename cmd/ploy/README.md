# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid and orchestrates follow-up jobs. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy workflow run --ticket <ticket-id>
```
At this stage the command validates the ticket flag and returns `ErrNotImplemented`. Subsequent roadmap items will connect the CLI to JetStream, resolve workflow DAGs, and publish job specs back to Grid.

## Flags
- `--ticket` — JetStream/Queue identifier to resolve. Required until automatic ticket discovery lands in `02-workflow-runner-cli`.

## Exit Codes
- `0` — success (future slices will signify completed workflows).
- `1` — error (missing ticket, unsupported subcommand, or stubbed implementation).

## Development
- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test`.
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI focused on stateless execution.
