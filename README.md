# Ploy — Stateless Workflow Runner (SHIFT)

Ploy is being reinvented as an on-demand workflow brain that consumes Grid events, resolves workflow DAGs, and exits once follow-up jobs are handed back to Grid. This repository now focuses exclusively on that CLI experience; all legacy API, Nomad, Consul, and SeaweedFS components have been removed as part of the SHIFT initiative.

## Current Status
- ✅ Repository reduced to CLI-only entrypoint (`ploy workflow run`).
- ✅ Legacy binaries, Nomad orchestration code, and SeaweedFS adapters removed.
- ✅ Event contract slice now supports live JetStream connections when ``JETSTREAM_URL`` is set (falling back to the in-memory stub).
- ✅ Lane engine exposes deterministic specs under `configs/lanes/*.toml` plus `ploy lanes describe` for cache previews.
- ✅ Snapshot toolkit slice ships `ploy snapshot plan` / `ploy snapshot capture`, applies strip/mask/synthetic rules locally, and publishes metadata to the in-memory JetStream/IPFS stubs.
- ✅ Integration manifest compiler validates TOML manifests under `configs/manifests/`, attaches compiled payloads to workflow stages, and enforces lane allowlists in the Grid stub.
- ✅ Recipe pack registry loads pluggable pack list specs from `configs/recipes/` for the OpenRewrite catalog, paving the way for Kotlin/Gradle support.
- ✅ Commit-scoped environment command (`ploy environment materialize`) assembles manifest fixtures, validates required snapshots, and hydrates lane caches via in-memory stubs.

## Getting Started
1. **Clone & build**
   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```

2. **Inspect lane metadata**
   ```bash
   ./dist/ploy lanes describe --lane go-native --commit HEAD --snapshot dev-db --manifest smoke --aster plan,exec
   ```
   The command parses `configs/lanes/go-native.toml`, previews the composed cache key, and lists the build/test commands bound to that lane.

3. **Run the workflow CLI**
   ```bash
   JETSTREAM_URL=nats://127.0.0.1:4222 ./dist/ploy workflow run --tenant acme --ticket auto
   ```
   With ``JETSTREAM_URL`` set the CLI connects to JetStream, claims the next ticket, and publishes checkpoints on the real stream. When omitted it boots the in-memory stub for offline development.

4. **Preview snapshot rules**
   ```bash
   ./dist/ploy snapshot plan --snapshot dev-db
   ```
   The plan command loads `configs/snapshots/dev-db.toml`, summarises strip/mask/synthetic rules, and highlights which tables/columns are affected before a capture runs.

5. **Capture a snapshot (stub)**
   ```bash
   ./dist/ploy snapshot capture --snapshot dev-db --tenant acme --ticket SNAPSHOT-1
   ```
   Capture applies the configured rules against `configs/snapshots/dev-db.json`, hashes the result, emits a fake IPFS CID, and publishes metadata to the JetStream stub.

6. **Dry-run a commit-scoped environment**
   ```bash
   ./dist/ploy environment materialize deadbeef --app commit-app --tenant acme --dry-run
   ```
   Dry-run mode compiles the `commit-app` manifest, verifies required snapshots (`commit-db`, `commit-cache`), and previews cache keys for each required lane without mutating state.

7. **Tests**
   ```bash
   make test
   ```
   Unit tests assert that only the workflow CLI remains and that the event contract schema stays consistent.

## Roadmap Alignment
The active roadmap lives under `roadmap/shift/`. Completed slices:
- [x] `00-legacy-teardown` — repository scoped to the stateless CLI stub.
- [x] `01-event-contracts` — subject map + schema definitions with a JetStream stub.
- [x] `02-workflow-runner-cli` — CLI reconstructs the default DAG, streams checkpoints, and exercises the Grid stub.
- [x] `03-lane-engine` — lane specs + cache key composer + `ploy lanes describe` inspection command.
- [x] `04-snapshot-toolkit` — snapshot commands, rule engine, and metadata publishing via JetStream/IPFS stubs.
- [x] `05-integration-manifests` — manifest compiler + Grid lane enforcement.
- [x] `06-commit-environments` — commit-scoped environment materialisation with dry-run/execute modes.
- [x] `07-aster-hook` — Aster bundle discovery, cache toggle plumbing, and CLI flags.
- [x] `08-documentation-cleanup` — doc set refreshed to highlight the CLI-first/Grid model.
- [x] `09-cache-coordination` — workflow checkpoints carry lane cache keys for Grid reuse.
- [x] `10-jetstream-client` — workflow runs connect to live JetStream when available and keep the stub fallback for offline slices.

Next up: replace the Grid stub with the real Workflow RPC client and exercise the runner against the Dev API once GRID integration resumes.

See `docs/design/shift/README.md` for the full design intent and sequencing.

## Environment Placeholders
Workstation builds still rely on the in-memory Grid stub; JetStream is now live-optional. Keep the following environment variables handy:
- ``JETSTREAM_URL`` — JetStream endpoint used by the workflow runner and snapshot publisher (optional; falls back to stub).
- ``GRID_ENDPOINT`` — Workflow RPC host used to submit jobs back to Grid.
- ``IPFS_GATEWAY`` — Gateway for retrieving snapshot artifacts published by `ploy snapshot capture`.

## Contributing
- Follow the instructions in `AGENTS.md` (TDD cadence, coverage expectations, VPS workflows).
- Keep documentation aligned with `docs/DOCS.md`.
- Each roadmap slice should land with RED → GREEN → REFACTOR (unit tests locally, integration tests via Grid once implemented).

## License
The project inherits its existing license terms; consult `LICENSE` if/when it is reintroduced in a future slice.
