# Event Contracts (Roadmap 01)

## Purpose
Capture the JetStream subject map and JSON schemas that let the Ploy CLI operate statelessly while Grid remains the control surface owner. This slice introduces workstation-only stubs so tickets can be claimed and checkpoints emitted without live JetStream wiring.

## Subject Map
- `grid.webhook.<tenant>` — Grid-owned inbox where tickets are queued for consumption.
- `ploy.workflow.<ticket>.checkpoints` — Ploy-published stream containing DAG checkpoints, cache signals, and retry markers.
- `ploy.artifact.<ticket>` — Ploy-published stream of IPFS hashes for build outputs, snapshot bundles, and diff reports.
- `grid.status.<ticket>` — Grid-owned stream reporting job lifecycle events that Ploy tails before exit.

## Message Schemas
- **WorkflowTicket** — minimal claim payload pulled from `grid.webhook.<tenant>`.
  ```json
  {
    "schema_version": "2025-09-26",
    "ticket_id": "ticket-123",
    "tenant": "acme"
  }
  ```
- **WorkflowCheckpoint** — checkpoint envelope published to `ploy.workflow.<ticket>.checkpoints`.
  ```json
  {
    "schema_version": "2025-09-26",
    "ticket_id": "ticket-123",
    "stage": "ticket-claimed",
    "status": "claimed",
    "cache_key": "go-native/go-native@commit=none@snapshot=none@manifest=2025-09-26@aster=plan"
  }
  ```

The constants live in `internal/workflow/contracts` (`SchemaVersion` et al.), ensuring the CLI and future Grid integrations consume identical versions. Checkpoints now carry the lane cache key so Grid can coordinate cache hydration and reuse.

## JetStream Client
- `internal/workflow/contracts.JetStreamClient` now implements `runner.EventsClient`, connecting to NATS when ``JETSTREAM_URL`` is provided and falling back to the in-memory bus for offline runs.
- `cmd/ploy/main.go` selects the real client automatically when the environment variable is set, closing the loop on the original stub pathway.
- `internal/workflow/contracts.InMemoryBus` remains available for workstation slices that skip live connectivity.
- `internal/workflow/grid.Client` now provides the Workflow RPC implementation toggled by ``GRID_ENDPOINT``; `internal/workflow/contracts.InMemoryBus` and the Grid stub remain for offline slices while `IPFS_GATEWAY` is still TODO until artifact publishing lands.

## Tests
- Unit tests in `internal/workflow/contracts` validate subject derivation, schema validation, and stub behaviour.
- Runner tests ensure the CLI claims tickets and publishes an initial `claimed` checkpoint through the stub.

## Next Steps
- Expand tickets and checkpoints to include DAG metadata and artifact manifests once the lane engine lands.
- ✅ Completed 2025-09-26: Wire the workflow runner to submit stages to Grid via the Workflow RPC so live runs exercise the real control plane.
