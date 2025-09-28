# Stage Artifact Streams (SHIFT Roadmap 18)

## Purpose
Mirror workflow stage artifact manifests onto the `ploy.artifact.<ticket>` JetStream subject so cache hydrators and downstream tooling can react without scraping checkpoints. This closes the follow-up listed in `docs/design/checkpoint-metadata/README.md` and aligns the workflow runner with the snapshot metadata stream introduced earlier.

## Scope
- Applies to the workflow runner (`internal/workflow/runner`) and events client implementations under `internal/workflow/contracts`.
- Workstation-only slice: `JETSTREAM_URL` enables real JetStream publishes; otherwise the in-memory stub records artifact messages for tests and local runs.
- Reuses the existing artifact subject set derived from `contracts.SubjectsForTenant`.

## Behaviour
- When a stage finishes with artifact manifests, the runner publishes a checkpoint **and** individual artifact envelopes to `ploy.artifact.<ticket>`.
- Each artifact envelope carries the schema version, ticket ID, stage name, optional cache key, stage metadata (lane, dependencies, manifest, and—when ``PLOY_ASTER_ENABLE`` is set—Aster), and a single artifact manifest (name/CID/digest/media type).
- Offline slices (no `JETSTREAM_URL`) persist envelopes in-memory for assertions; live JetStream runs publish JSON messages for consumers.
- Workflow-level checkpoints remain unchanged; artifact envelopes are only emitted for stage-level completions with artifacts.

## Implementation Notes
- Introduce `contracts.WorkflowArtifact` with validation helpers and a `Subject()` method resolving the artifact stream for the ticket.
- Extend `runner.EventsClient` with `PublishArtifact` and implement it in both the in-memory bus and JetStream client (`contracts.jetstream`).
- Update `runner.publishCheckpoint` so completed stages dispatch artifact envelopes after checkpoint publication; skip envelopes when artifacts list is empty or invalid.
- Ensure artifact envelopes reuse `contracts.CheckpointStage`/`CheckpointArtifact` for consistent validation.
- Keep `GRID_ENDPOINT` and `IPFS_GATEWAY` handling unchanged; this slice only touches the event path.

## Tests
- Runner tests asserting artifact envelopes are emitted for completed stages and absent for non-completed checkpoints.
- Contracts tests covering validation/marshalling for the new envelope, in-memory bus recording, and JetStream publication against an in-process NATS server.
- Repository-wide `go test -cover ./...` to maintain ≥60% overall and ≥90% coverage on workflow runner packages.

## Rollout & Follow-ups
- Update `docs/design/checkpoint-metadata/README.md` to mark the artifact stream follow-up complete.
- Record status in `roadmap/shift/18-stage-artifact-streams.md` once the slice lands.
- Future slice: attach artifact payload hashing/size metadata when Grid begins returning those fields.
