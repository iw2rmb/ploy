# Ploy — Stateless Workflow Runner (SHIFT)

Ploy is being reinvented as an on-demand workflow brain that consumes Grid events, resolves workflow DAGs, and exits once follow-up jobs are handed back to Grid. This repository now focuses exclusively on that CLI experience; all legacy API, Nomad, Consul, and SeaweedFS components have been removed as part of the SHIFT initiative.

## Current Status
- ✅ Repository reduced to CLI-only entrypoint (`ploy workflow run`).
- ✅ Legacy binaries, Nomad orchestration code, and SeaweedFS adapters removed.
- ✅ Event contract scaffolding in place: the CLI claims a ticket and publishes checkpoints via the JetStream stub.
- ✅ Lane engine exposes deterministic specs under `configs/lanes/*.toml` plus `ploy lanes describe` for cache previews.
- ✅ Snapshot toolkit slice ships `ploy snapshot plan` / `ploy snapshot capture`, applies strip/mask/synthetic rules locally, and publishes metadata to the in-memory JetStream/IPFS stubs.
- ✅ Integration manifest compiler validates TOML manifests under `configs/manifests/`, attaches compiled payloads to workflow stages, and enforces lane allowlists in the Grid stub.

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

3. **Run the workflow CLI stub**
   ```bash
   ./dist/ploy workflow run --tenant acme --ticket TICKET-123
   ```
   The command hydrates the event contract stub, claims the ticket, and publishes a `claimed` checkpoint locally.

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

6. **Tests**
   ```bash
   make test
   ```
   Unit tests assert that only the workflow CLI remains and that the event contract schema stays consistent.

## Roadmap Alignment
The active roadmap lives under `roadmap/shift/`. Completed items:
- [x] `00-legacy-teardown` — repository scoped to CLI-only workflow runner stub.
- [x] `01-event-contracts` — subject map + schema definitions with a stubbed JetStream client.
- [x] `02-workflow-runner-cli` — CLI reconstructs the default DAG, streams checkpoints, and exercises the Grid stub.
- [x] `03-lane-engine` — lane specs + cache key composer + `ploy lanes describe` inspection command.

Upcoming items:
- `06-commit-environments` — hydrate caches/snapshots/manifests for commit-scoped runs.
- `07-aster-hook` — wire AST-pruned bundles into cache keys and Grid submissions.

See `docs/design/shift/README.md` for the full design intent and sequencing.

## Environment Placeholders
Workstation builds still rely on in-memory stubs; the real services land once JetStream/Grid wiring resumes. Keep the following environment variables handy (currently marked TODO until integration testing moves off the workstation):
- ``JETSTREAM_URL`` — JetStream endpoint used by the workflow runner and snapshot publisher.
- ``GRID_ENDPOINT`` — Workflow RPC host used to submit jobs back to Grid.
- ``IPFS_GATEWAY`` — Gateway for retrieving snapshot artifacts published by `ploy snapshot capture`.

## Contributing
- Follow the instructions in `AGENTS.md` (TDD cadence, coverage expectations, VPS workflows).
- Keep documentation aligned with `docs/DOCS.md`.
- Each roadmap slice should land with RED → GREEN → REFACTOR (unit tests locally, integration tests via Grid once implemented).

## License
The project inherits its existing license terms; consult `LICENSE` if/when it is reintroduced in a future slice.
