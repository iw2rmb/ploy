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
    "schema_version": "2025-09-25",
    "ticket_id": "ticket-123",
    "tenant": "acme"
  }
  ```
- **WorkflowCheckpoint** — checkpoint envelope published to `ploy.workflow.<ticket>.checkpoints`.
  ```json
  {
    "schema_version": "2025-09-25",
    "ticket_id": "ticket-123",
    "stage": "ticket-claimed",
    "status": "claimed"
  }
  ```

The constants live in `internal/workflow/contracts` (`SchemaVersion` et al.), ensuring the CLI and future Grid integrations consume identical versions.

## Stubbed JetStream Client
- `internal/workflow/contracts.InMemoryBus` implements `runner.EventsClient` and records claimed tickets and checkpoints.
- `cmd/ploy/main.go` wires the stub for workstation runs; real JetStream connectivity lands in `02-workflow-runner-cli`.
- `GRID_ENDPOINT`, `JETSTREAM_URL`, and `IPFS_GATEWAY` remain unset in this slice. Note them as TODOs for the workflow runner wiring once JetStream integration resumes.

## Tests
- Unit tests in `internal/workflow/contracts` validate subject derivation, schema validation, and stub behaviour.
- Runner tests ensure the CLI claims tickets and publishes an initial `claimed` checkpoint through the stub.

## Next Steps
- Replace the stub with a real JetStream client (respecting `JETSTREAM_URL`) and extend checkpoints with cache key payloads.
- Expand tickets and checkpoints to include DAG metadata and artifact manifests once the lane engine lands.
