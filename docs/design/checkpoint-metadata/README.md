# Workflow Checkpoint Metadata (Roadmap 17)

## Purpose

Carry full DAG context and artifact manifests inside workflow checkpoints so
Grid operators and downstream tooling can introspect stage lane assignments,
dependencies, and produced artifacts without scraping CLI output. This closes
the follow-up item from `docs/design/event-contracts/README.md` to enrich
checkpoint payloads now that the lane engine and artifact publishers exist.

## Scope

- Applies to the workflow runner (`internal/workflow/runner`), contracts
  (`internal/workflow/contracts`), and Grid client (`internal/workflow/grid`).
- Workstation-only slice: events still publish through JetStream when discovery
  returns routes, but validation focuses on the in-memory bus and unit tests.
- Extends the existing checkpoint schema (no new subjects) by embedding stage
  and artifact metadata alongside status transitions.

## Behaviour

- Every checkpoint published for a stage includes a `stage_metadata` block
  describing the stage name, kind, lane, dependencies, manifest reference, and
  (when `PLOY_ASTER_ENABLE` is set) the active Aster toggles/bundles.
- Stage-completed checkpoints include an `artifacts` list derived from Grid
  stage outcomes so cache hydrators can react to produced bundles immediately.
  Retry/pending checkpoints omit artifacts.
- Cache keys remain present for stages, and workflow-level checkpoints continue
  to render without stage metadata blocks.
- Clients consuming the event stream can rely on the schema version bump to
  detect the richer payloads.

## Implementation Notes

- Introduce `contracts.CheckpointStage`, `contracts.CheckpointStageAster`, and
  `contracts.CheckpointArtifact` structs; embed optional pointers/arrays on
  `WorkflowCheckpoint`.
- Expand `runner.publishCheckpoint` so stage checkpoints include the
  `CheckpointStage` payload composed from the planner output and manifest
  compilation.
- Extend `runner.StageOutcome` with an `Artifacts` slice; update the in-memory
  Grid stub and Grid HTTP client to propagate artifact manifests.
- Update the JetStream and in-memory contracts clients to validate and marshal
  the enriched checkpoint structure.
- Bump the workflow schema version constant to reflect the new payload contract
  and document the change.

## Tests

- New/updated unit tests in `internal/workflow/contracts` covering
  validation/marshalling of stage and artifact payloads.
- Runner tests asserting checkpoints provide stage metadata, cache keys, and
  artifacts for completed stages while workflow-level checkpoints remain lean.
- Grid client tests asserting artifact manifests round-trip through the HTTP
  response into runner checkpoints.
- Repository-wide `go test -cover ./...` to maintain ≥60% coverage overall and
  ≥90% in the runner package.
- Keep RED → GREEN → REFACTOR in play: fail checkpoint enrichment tests first,
  add minimal struct wiring, then refactor after coverage stabilises.

## Rollout & Follow-ups

- Update `docs/design/event-contracts/README.md` to mark the checkpoint metadata
  follow-up complete and describe the enriched schema.
- Add roadmap entry `docs/tasks/roadmap/17-checkpoint-metadata.md` and mark it done
  when this slice ships.
- ✅ Completed 2025-09-26: Mirror stage artifacts to the
  `ploy.artifact.<ticket>` subject once build artifact uploads move off the
  workstation stub.
