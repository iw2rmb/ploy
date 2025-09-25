# Ploy — Stateless Workflow Runner (SHIFT)

Ploy is being reinvented as an on-demand workflow brain that consumes Grid events, resolves workflow DAGs, and exits once follow-up jobs are handed back to Grid. This repository now focuses exclusively on that CLI experience; all legacy API, Nomad, Consul, and SeaweedFS components have been removed as part of the SHIFT initiative.

## Current Status
- ✅ Repository reduced to CLI-only entrypoint (`ploy workflow run`).
- ✅ Legacy binaries, Nomad orchestration code, and SeaweedFS adapters removed.
- ✅ Event contract scaffolding in place: the CLI claims a ticket and publishes an initial checkpoint via the JetStream stub.
- 🚧 Next roadmap slices will connect to real JetStream, layer in the lane engine, snapshot tooling, and integration manifests.

## Getting Started
1. **Clone & build**
   ```bash
   git clone https://github.com/iw2rmb/ploy
   cd ploy
   make build
   ```

2. **Run the CLI stub**
   ```bash
   ./dist/ploy workflow run --tenant acme --ticket TICKET-123
   ```
   The command hydrates the event contract stub, claims the ticket, and publishes a `claimed` checkpoint locally.

3. **Tests**
   ```bash
   make test
   ```
   Unit tests assert that only the workflow CLI remains and that the event contract schema stays consistent.

## Roadmap Alignment
The active roadmap lives under `roadmap/shift/`. Completed items:
- [x] `00-legacy-teardown` — repository scoped to CLI-only workflow runner stub.
- [x] `01-event-contracts` — subject map + schema definitions with a stubbed JetStream client.

Upcoming items:
- `02-workflow-runner-cli` — connect the CLI to JetStream, reconstruct DAGs, and submit Grid jobs.
- `03-lane-engine` onward — lanes, snapshot tooling, integration manifests, commit-scoped environments, and Aster hook integration.

See `docs/design/shift/README.md` for the full design intent and sequencing.

## Contributing
- Follow the instructions in `AGENTS.md` (TDD cadence, coverage expectations, VPS workflows).
- Keep documentation aligned with `docs/DOCS.md`.
- Each roadmap slice should land with RED → GREEN → REFACTOR (unit tests locally, integration tests via Grid once implemented).

## License
The project inherits its existing license terms; consult `LICENSE` if/when it is reintroduced in a future slice.
