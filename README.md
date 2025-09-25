# Ploy — Stateless Workflow Runner (SHIFT)

Ploy is being reinvented as an on-demand workflow brain that consumes Grid events, resolves workflow DAGs, and exits once follow-up jobs are handed back to Grid. This repository now focuses exclusively on that CLI experience; all legacy API, Nomad, Consul, and SeaweedFS components have been removed as part of the SHIFT initiative.

## Current Status
- ✅ Repository reduced to CLI-only entrypoint (`ploy workflow run`).
- ✅ Legacy binaries, Nomad orchestration code, and SeaweedFS adapters removed.
- ✅ Documentation rewritten to describe the Grid-first model.
- 🚧 Workflow runner currently returns `ErrNotImplemented` after validating tickets. Subsequent roadmap items layer in JetStream contracts, lane engine, snapshot tooling, and integration manifests.

## Getting Started
1. **Clone & build**
   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```

2. **Run the CLI stub**
   ```bash
   ./dist/ploy workflow run --ticket TICKET-123
   ```
   The command validates the ticket flag and exits with `ErrNotImplemented` while downstream integrations are implemented.

3. **Tests**
   ```bash
   make test
   ```
   Unit tests assert that only the workflow CLI remains and that the legacy dependency surface is gone.

## Roadmap Alignment
The active roadmap lives under `roadmap/shift/`. Completed item:
- [x] `00-legacy-teardown` — repository scoped to CLI-only workflow runner stub.

Upcoming items:
- `01-event-contracts` — define JetStream subjects and schemas shared with Grid.
- `02-workflow-runner-cli` — connect the CLI to JetStream, reconstruct DAGs, and submit Grid jobs.
- `03-lane-engine` onward — lanes, snapshot tooling, integration manifests, commit-scoped environments, and Aster hook integration.

See `docs/design/shift/README.md` for the full design intent and sequencing.

## Contributing
- Follow the instructions in `AGENTS.md` (TDD cadence, coverage expectations, VPS workflows).
- Keep documentation aligned with `docs/DOCS.md`.
- Each roadmap slice should land with RED → GREEN → REFACTOR (unit tests locally, integration tests via Grid once implemented).

## License
The project inherits its existing license terms; consult `LICENSE` if/when it is reintroduced in a future slice.
