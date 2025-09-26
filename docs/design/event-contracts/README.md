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
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "tenant": "acme"
  }
  ```
- **WorkflowCheckpoint** — checkpoint envelope published to `ploy.workflow.<ticket>.checkpoints`.
  ```json
  {
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "stage": "mods",
    "status": "running",
    "cache_key": "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
    "stage_metadata": {
      "name": "mods",
      "kind": "mods",
      "lane": "node-wasm",
      "manifest": {"name": "smoke", "version": "2025-09-26"},
      "dependencies": [],
      "aster": {
        "enabled": true,
        "toggles": ["plan"],
        "bundles": [
          {
            "stage": "mods",
            "toggle": "plan",
            "bundle_id": "mods-plan",
            "artifact_cid": "cid-mods-plan",
            "digest": "sha256:modsplan"
          }
        ]
      }
    },
    "artifacts": [
      {
        "name": "mods-plan",
        "artifact_cid": "cid-mods-plan",
        "digest": "sha256:modsplan",
        "media_type": "application/tar+zst"
      }
    ]
  }
  ```
- **WorkflowArtifact** — stage artifact envelope mirrored to `ploy.artifact.<ticket>` whenever a stage completes with artifact manifests.
  ```json
  {
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "stage": "mods",
    "cache_key": "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
    "stage_metadata": {
      "name": "mods",
      "kind": "mods",
      "lane": "node-wasm",
      "manifest": {"name": "smoke", "version": "2025-09-26"},
      "aster": {"enabled": true, "toggles": ["plan"]}
    },
    "artifact": {
      "name": "mods-plan",
      "artifact_cid": "cid-mods-plan",
      "digest": "sha256:modsplan",
      "media_type": "application/tar+zst"
    }
  }
  ```

The constants live in `internal/workflow/contracts` (`SchemaVersion` et al.), ensuring the CLI and future Grid integrations consume identical versions. Checkpoints now carry lane cache keys, stage metadata, and optional artifact manifests so Grid can coordinate cache reuse and artifact hydration.

## JetStream Client
- `internal/workflow/contracts.JetStreamClient` now implements `runner.EventsClient`, connecting to NATS when ``JETSTREAM_URL`` is provided and falling back to the in-memory bus for offline runs.
- `cmd/ploy/main.go` selects the real client automatically when the environment variable is set, closing the loop on the original stub pathway.
- `internal/workflow/contracts.InMemoryBus` remains available for workstation slices that skip live connectivity.
- `internal/workflow/grid.Client` now provides the Workflow RPC implementation toggled by ``GRID_ENDPOINT``; `internal/workflow/contracts.InMemoryBus` and the Grid stub remain for offline slices while `IPFS_GATEWAY` is still TODO until artifact publishing lands.

## Tests
- Unit tests in `internal/workflow/contracts` validate subject derivation, schema validation, and stub behaviour.
- Runner tests ensure the CLI claims tickets and publishes an initial `claimed` checkpoint through the stub.

## Next Steps
- ✅ Completed 2025-09-26: Expand checkpoints with stage metadata and artifact manifests (see `docs/design/checkpoint-metadata/README.md`).
- ✅ Completed 2025-09-26: Wire the workflow runner to submit stages to Grid via the Workflow RPC so live runs exercise the real control plane.
- ✅ Completed 2025-09-26: Mirror workflow stage artifact envelopes to `ploy.artifact.<ticket>` via the new event contract.
