# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid, reconstructs the default mods→build→test DAG, and dispatches stages to the in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy lanes describe --lane <lane-name> [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] [--aster <toggle,...>]
ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]
```
`lanes describe` inspects TOML lane specs under `configs/lanes/`, displays the runtime family, build/test commands, and shows a deterministic cache-key preview that incorporates commit/snapshot/manifest/Aster toggles. The preview mirrors what the workflow runner supplies to Grid when dispatching stages.

`workflow run` boots in-memory JetStream and Grid stubs, claims a ticket (auto-generating one if `--ticket auto`), publishes checkpoints for every stage transition, executes mods/build/test against a temporary workspace, and cleans up before exit. Upcoming roadmap slices will swap the stubs for real JetStream connections and Grid RPC calls.

## Flags
- `--lane` — Lane identifier defined under `configs/lanes/*.toml` (required for `lanes describe`).
- `--commit` / `--snapshot` / `--manifest` / `--aster` — Optional cache-key preview inputs consumed by the lane engine.
- `--tenant` — Tenant slug used to resolve subject namespaces. Required for `workflow run`.
- `--ticket` — JetStream ticket identifier to claim (`workflow run`). Defaults to `auto`, which selects or generates the next ticket on the tenant stub.

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
- Run unit tests with `make test` (ensures `go test -cover ./...` stays ≥60% overall, ≥90% on the runner package).
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI focused on stateless execution against JetStream/Grid contracts.
