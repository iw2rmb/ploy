# Stage Artifact Streams
- [x] Done (2025-09-26)

## Why / What For
Give downstream consumers immediate access to workflow stage artifact manifests via JetStream, eliminating the need to scrape checkpoints or CLI logs while keeping workstation slices functional without live infrastructure.

## Required Changes
- Define a workflow artifact envelope in `internal/workflow/contracts` and expose it through the events client interface.
- Update the workflow runner to publish artifact envelopes for completed stages alongside checkpoints.
- Extend the JetStream client and in-memory bus to emit/store artifact envelopes while maintaining the existing checkpoint behaviour.

## Definition of Done
- Stage completions with artifacts publish envelopes to `ploy.artifact.<ticket>` (JetStream) or the in-memory bus; stages without artifacts produce no envelopes.
- Artifact envelopes carry schema version, ticket ID, stage metadata, cache key, and manifest details consistent with checkpoints.
- Workflow-level checkpoints remain unchanged and do not duplicate artifact lists.
- Documentation (`docs/design/checkpoint-metadata/README.md`, `docs/design/stage-artifacts/README.md`) reflects the behaviour and roadmap entry is marked complete.

Status: Stage completions now emit JSON artifact envelopes to JetStream (or the in-memory stub) alongside checkpoints, satisfying the cache hydrator requirements without altering workflow-level checkpoints.

## Tests
- Runner unit tests verifying artifact envelopes are emitted exactly once per artifact on stage completion.
- Contracts unit tests covering validation/marshalling, in-memory storage, and JetStream publication for artifact envelopes.
- `go test -cover ./...` meets repository coverage thresholds (≥60% overall, ≥90% runner package).
