# Ploy Workflow CLI

`ploy` is now a single-purpose CLI that claims workflow tickets from Grid, reconstructs the default mods→build→test DAG, and dispatches stages to the in-memory Grid stub. Legacy subcommands (apps, env, mods, security, etc.) were removed during the SHIFT legacy teardown.

## Usage
```
ploy lanes describe --lane <lane-name> [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] [--aster <toggle,...>]
ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--aster <toggle,...>] [--aster-step <stage=toggle,...|stage=off>]
ploy snapshot plan --snapshot <snapshot-name>
ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>
ploy environment materialize <commit-sha> --app <app> --tenant <tenant> [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]
```
`lanes describe` inspects TOML lane specs under `configs/lanes/`, displays the runtime family, build/test commands, and shows a deterministic cache-key preview that incorporates commit/snapshot/manifest/Aster toggles. The preview mirrors what the workflow runner supplies to Grid when dispatching stages.

`workflow run` connects to JetStream when ``JETSTREAM_URL`` is set (falling back to the in-memory stub otherwise), claims a ticket (auto-generating one if `--ticket auto`), compiles the referenced integration manifest from `configs/manifests/`, publishes checkpoints for every stage transition (now including lane cache keys), executes mods/build/test against a temporary workspace, and cleans up before exit. The Grid stub still backs stage execution for this slice and refuses stages whose lanes are not declared in the manifest. Aster bundle provenance is surfaced after a successful run so developers can confirm which toggles/bundles were attached to each stage. Explicit ticket IDs remain a stub-only workflow until Grid integration lands.

`snapshot plan` inspects TOML specs under `configs/snapshots/`, counting strip/mask/synthetic rules and surfacing per-table highlights before a capture runs.

`snapshot capture` loads the fixture referenced in the spec, applies strip/mask/synthetic rules, produces a deterministic fingerprint, publishes artifact metadata to the JetStream stub, and returns the fake IPFS CID assigned by the in-memory publisher.

`environment materialize` evaluates the integration manifest for a given app/commit pair, validates required snapshots, optionally captures them (execution mode), composes deterministic cache keys for each required lane, and hydrates those caches through an in-memory hydrator. Dry-run mode avoids snapshot capture/hydration and surfaces any gaps before Grid integration lands.

## Flags
- `--lane` — Lane identifier defined under `configs/lanes/*.toml` (required for `lanes describe`).
- `--commit` / `--snapshot` / `--manifest` / `--aster` — Optional cache-key preview inputs consumed by the lane engine.
- `--tenant` — Tenant slug used to resolve subject namespaces. Required for `workflow run`, `snapshot capture`, and execution-mode `environment materialize`.
- `--ticket` — JetStream ticket identifier to claim (`workflow run`) or metadata tag for snapshot captures. Defaults to `auto` for workflows; required for snapshot captures.
- `--snapshot` — Snapshot identifier defined under `configs/snapshots/*.toml` (required for `snapshot plan` and `snapshot capture`).
- `--app` — Application identifier resolved to an integration manifest (required for `environment materialize`).
- `--dry-run` — Skip snapshot capture and cache hydration while still reporting required resources (`environment materialize`).
- `--manifest` — Override manifest name/version in `<name>@<version>` form (`environment materialize`).
- `--aster` — Optional toggles to append to manifest-required Aster switches (`lanes describe`, `workflow run`, `environment materialize`).
- `--aster-step` — Stage-specific overrides for Aster behaviour when running workflows (`workflow run`). Use `stage=toggle1,toggle2` to enable additional toggles or `stage=off` to disable Aster for that stage.

## Exit Codes
- `0` — success (ticket claimed, stages completed, workspace cleaned).
- `1` — error (missing flags, unsupported subcommand, stage failure, or downstream error).

## Environment
- ``JETSTREAM_URL`` — NATS/JetStream endpoint (`nats://host:port`) used by `workflow run` when present.
- ``GRID_ENDPOINT`` — TODO (real Workflow RPC target lands with Grid integration).
- ``IPFS_GATEWAY`` — TODO (snapshot/artifact publishing slice).
When ``JETSTREAM_URL`` is omitted the CLI falls back to the in-memory stub so workstation tests continue to run offline.

## Development
- Build via `make build` (outputs to `dist/ploy`).
- Run unit tests with `make test` (ensures `go test -cover ./...` stays ≥60% overall, ≥90% on the runner package).
- Roadmap slices should extend `internal/workflow/runner` and keep the CLI focused on stateless execution against JetStream/Grid contracts.
- See `docs/MANIFESTS.md` for schema details and authoring guidance on integration manifests.
- Review `docs/DOCS.md` for the documentation matrix and editing conventions that keep the CLI guides aligned.
