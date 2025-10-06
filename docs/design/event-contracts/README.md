# Event Contracts (Roadmap 01)

## Purpose

Capture the JetStream subject map and JSON schemas that let the Ploy CLI operate
statelessly while Grid remains the control surface owner. This slice introduces
workstation-only stubs so tickets can be claimed and checkpoints emitted without
live JetStream wiring.

## Subject Map

- `webhook.<tenant>.<source>.<event>` — Grid-owned inbox where tickets and
  workflow intents are emitted per provider/event. Ploy uses `source=ploy` and
  `event=workflow-ticket` for Workflow RPC submissions
  (`webhook.<tenant>.ploy.workflow-ticket`).
- `ploy.workflow.<ticket>.checkpoints` — Ploy-published stream containing DAG
  checkpoints, cache signals, and retry markers.
- `ploy.artifact.<ticket>` — Ploy-published stream of IPFS hashes for build
  outputs, snapshot bundles, and diff reports.
- `jobs.<run_id>.events` — Grid-owned stream reporting workflow lifecycle events
  that Ploy tails before exit.

## Message Schemas

- **WorkflowTicket** — minimal claim payload pulled from
  `webhook.<tenant>.<source>.<event>`.

  ```json
  {
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "tenant": "acme",
    "repo": {
      "url": "https://gitlab.com/acme/repo.git",
      "base_ref": "main",
      "target_ref": "mods/workflow-123",
      "workspace_hint": "mods/java"
    }
  }
  ```

- **WorkflowCheckpoint** — checkpoint envelope published to
  `ploy.workflow.<ticket>.checkpoints`.

  ```json
  {
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "stage": "mods-plan#heal1",
    "status": "running",
    "cache_key": "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
    "stage_metadata": {
      "name": "mods-plan#heal1",
      "kind": "mods-plan",
      "lane": "mods-plan",
      "manifest": { "name": "smoke", "version": "2025-09-26" },
      "dependencies": [],
      "mods": {
        "plan": {
          "selected_recipes": ["org.openrewrite.java.UpgradeJavaVersion"],
          "parallel_stages": ["orw-apply", "orw-gen"],
          "human_gate": true,
          "summary": "Retry OpenRewrite with KB suggestions",
          "plan_timeout": "2m30s",
          "max_parallel": 2
        },
        "recommendations": [
          { "source": "knowledge-base", "message": "Apply rewrite recipe for JDK17", "confidence": 0.82 }
        ],
        "human": {
          "required": true,
          "playbooks": ["playbooks/mods/manual-check.md"]
        }
      },
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

- **WorkflowArtifact** — stage artifact envelope mirrored to
  `ploy.artifact.<ticket>` whenever a stage completes with artifact manifests.

  ```json
  {
    "schema_version": "2025-09-26.1",
    "ticket_id": "ticket-123",
    "stage": "mods-plan#heal1",
    "cache_key": "node-wasm/node-wasm@manifest=2025-09-26@aster=plan",
    "stage_metadata": {
      "name": "mods-plan#heal1",
      "kind": "mods-plan",
      "lane": "mods-plan",
      "manifest": { "name": "smoke", "version": "2025-09-26" },
      "mods": {
        "plan": {
          "selected_recipes": ["org.openrewrite.java.UpgradeJavaVersion"],
          "parallel_stages": ["orw-apply", "orw-gen"],
          "human_gate": true,
          "summary": "Retry OpenRewrite with KB suggestions",
          "plan_timeout": "2m30s",
          "max_parallel": 2
        },
        "recommendations": [
          { "source": "knowledge-base", "message": "Apply rewrite recipe for JDK17", "confidence": 0.82 }
        ]
      },
      "aster": { "enabled": true, "toggles": ["plan"] }
    },
    "artifact": {
      "name": "mods-plan",
      "artifact_cid": "cid-mods-plan",
      "digest": "sha256:modsplan",
      "media_type": "application/tar+zst"
    }
  }
  ```

`stage_metadata.mods` mirrors planner output for both initial and healing
branches, while the `#healN` suffix on stage names lets consumers correlate
follow-up attempts with their parent build gate failures.

The constants live in `internal/workflow/contracts` (`SchemaVersion` et al.),
ensuring the CLI and future Grid integrations consume identical versions.
Checkpoints now carry lane cache keys, stage metadata, and optional artifact
manifests so Grid can coordinate cache reuse and artifact hydration.

## JetStream Client

- `internal/workflow/contracts.JetStreamClient` now implements
  `runner.EventsClient`, connecting to NATS when discovery returns JetStream
  routes and falling back to the in-memory bus for offline runs.
- `cmd/ploy/main.go` selects the real client automatically when discovery
  exposes routes, closing the loop on the original stub pathway.
- `internal/workflow/contracts.InMemoryBus` remains available for workstation
  slices that skip live connectivity.
- `internal/workflow/grid.Client` will migrate to the Workflow RPC SDK (see
  `docs/design/workflow-rpc-alignment/README.md`); until Roadmap 22 lands it
  continues to use the legacy stage stub for workstation slices.

## Tests

- Unit tests in `internal/workflow/contracts` validate subject derivation,
  schema validation, and stub behaviour.
- Runner tests ensure the CLI claims tickets and publishes an initial `claimed`
  checkpoint through the stub.
- Keep RED → GREEN → REFACTOR active: start with failing contract tests, add
  minimal schema updates, then refactor after coverage stabilises.

## References

- Grid Webhook Gateway design (`../grid/docs/design/webhook-gateway/README.md`)
  defines webhook subject publishing.
- Grid Jobs service event stream design (`../grid/docs/design/jobs/README.md`)
  details `jobs.<run_id>.events` emission.

## Verification (2025-09-27)

- Verified webhook subjects are published as `webhook.<tenant>.<source>.<event>`
  in `../grid/internal/webhook/gateway.go`.
- Verified job lifecycle events emit on `jobs.<run_id>.events` within
  `../grid/internal/jobs/publisher_jetstream.go`.
- 2025-09-29: Confirmed discovery-backed JetStream selection via
  `cmd/ploy/dependencies.go` and `cmd/ploy/workflow_run_grid_test.go`.

## Next Steps

- ✅ Completed 2025-09-26: Expand checkpoints with stage metadata and artifact
  manifests (see `docs/design/checkpoint-metadata/README.md`).
- ✅ Completed 2025-09-26: Wire the workflow runner to submit stages to Grid via
  the Workflow RPC so live runs exercise the real control plane.
- ✅ Completed 2025-09-26: Mirror workflow stage artifact envelopes to
  `ploy.artifact.<ticket>` via the new event contract.
