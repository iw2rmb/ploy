# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid, reconstructs the default mods→build→test DAG, and dispatches stages to the in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy lanes describe --lane <lane-name> [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] [--aster <toggle,...>]
ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]
ploy snapshot plan --snapshot <snapshot-name>
ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>
```
`lanes describe` inspects TOML lane specs under `configs/lanes/`, displays the runtime family, build/test commands, and shows a deterministic cache-key preview that incorporates commit/snapshot/manifest/Aster toggles. The preview mirrors what the workflow runner supplies to Grid when dispatching stages.

`workflow run` boots in-memory JetStream and Grid stubs, claims a ticket (auto-generating one if `--ticket auto`), compiles the referenced integration manifest from `configs/manifests/`, publishes checkpoints for every stage transition, executes mods/build/test against a temporary workspace, and cleans up before exit. The Grid stub refuses stages whose lanes are not declared in the manifest. Upcoming roadmap slices will swap the stubs for real JetStream connections and Grid RPC calls.

`snapshot plan` inspects TOML specs under `configs/snapshots/`, counting strip/mask/synthetic rules and surfacing per-table highlights before a capture runs.

`snapshot capture` loads the fixture referenced in the spec, applies strip/mask/synthetic rules, produces a deterministic fingerprint, publishes artifact metadata to the JetStream stub, and returns the fake IPFS CID assigned by the in-memory publisher.

## Flags
- `--lane` — Lane identifier defined under `configs/lanes/*.toml` (required for `lanes describe`).
- `--commit` / `--snapshot` / `--manifest` / `--aster` — Optional cache-key preview inputs consumed by the lane engine.
- `--tenant` — Tenant slug used to resolve subject namespaces. Required for `workflow run` and `snapshot capture`.
- `--ticket` — JetStream ticket identifier to claim (`workflow run`) or metadata tag for snapshot captures. Defaults to `auto` for workflows; required for snapshot captures.
- `--snapshot` — Snapshot identifier defined under `configs/snapshots/*.toml` (required for `snapshot plan` and `snapshot capture`).

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
- See `docs/MANIFESTS.md` for schema details and authoring guidance on integration manifests.
