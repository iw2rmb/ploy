# Checkpoint Metadata Enrichment

- [x] Done (2025-09-26)

## Why / What For

Ensure workflow checkpoints expose full DAG context (lane, dependencies,
manifest, Aster metadata) and produced artifact manifests so Grid consumers can
reason about cache reuse and artifact availability without bespoke CLI scraping.

## Required Changes

- Expand the workflow checkpoint schema with a `stage_metadata` payload and
  optional `artifacts` list.
- Update the workflow runner to attach stage metadata and bubble artifact
  manifests from Grid stage outcomes into checkpoints.
- Extend the Grid HTTP client and in-memory stub to capture and propagate
  artifact manifest data.
- Refresh design docs to capture the new schema and mark the outstanding
  follow-up complete.

## Definition of Done

- Stage checkpoints include stage metadata and cache keys; completed stage
  checkpoints also list any returned artifacts.
- Workflow-level checkpoints remain unchanged (no stage metadata/artifacts) to
  preserve summary semantics.
- Tests cover schema validation, runner checkpoint contents, and Grid client
  artifact propagation.
- Documentation reflects the enriched checkpoint schema and roadmap task is
  marked done.

## Tests

- `go test ./internal/workflow/contracts` validating checkpoint payload
  structure.
- `go test ./internal/workflow/runner` covering stage metadata/artifact emission
  and cache keys.
- `go test ./internal/workflow/grid` ensuring artifact manifests round-trip.
- Repository-wide `go test -cover ./...` maintaining ≥60% overall coverage.
