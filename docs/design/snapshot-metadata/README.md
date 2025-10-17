# Snapshot Metadata Streams (Roadmap 16)

## Purpose

Publish snapshot capture metadata to JetStream so Grid can ingest fingerprint,
CID, and rule counts without relying on the CLI stdout. This slice closes the
TODO called out in `docs/design/ipfs-artifacts/README.md` and aligns with the
roadmap contract that `ploy snapshot capture` pushes metadata to JetStream
alongside artifact uploads.

## Scope

- Applies to the snapshot toolkit (`internal/workflow/snapshots`) and the CLI
  wiring in `cmd/ploy`.
- Workstation-only: relies on Grid discovery returning JetStream routes. When
  discovery omits routes, the in-memory publisher remains the default.
- Uses the existing artifact stream subject (`ploy.artifact.<ticket>`) defined
  in `internal/workflow/contracts`.

## Behaviour

- Snapshot metadata is encoded as JSON with the global schema version, capture
  fingerprint, CID, rule counts, and timestamps.
- Messages are published to `ploy.artifact.<ticket>` for the active
  ticket/tenant. Consumers can replay and hydrate snapshot metadata
  independently of the CLI run output.
- When discovery omits JetStream routes the CLI continues to use the in-memory
  metadata publisher, preserving deterministic tests and offline slices.
- Errors from the JetStream publisher bubble up through `Capture`, giving
  operators actionable feedback.

## Implementation Notes

- Introduce `snapshots.NewJetStreamMetadataPublisher` that dials JetStream,
  caches the JetStream context, and publishes envelopes per metadata capture.
- Extend `SnapshotMetadata` emission to include the global schema version when
  publishing to JetStream. Preserve the existing struct for in-memory use so
  tests remain stable.
- `cmd/ploy` reads discovery output when loading the snapshot registry. When
  JetStream routes are returned it injects the metadata publisher alongside any
  discovered IPFS gateway.
- Add lightweight validation in the publisher (non-empty tenant, ticket, CID)
  before publishing.

## Tests

- Unit test in `internal/workflow/snapshots` using an in-process JetStream
  server to verify metadata publications land on `ploy.artifact.<ticket>` with
  the expected payload.
- CLI-level test ensuring the registry loader selects the JetStream metadata
  publisher when discovery returns routes and falls back to the no-op publisher
  otherwise.
- Repository-wide `go test -cover ./...` to keep ≥60% overall coverage and ≥90%
  in the snapshot package.
- Reinforce RED → GREEN → REFACTOR: create failing JetStream metadata tests, add
  minimal publisher wiring, then refactor once coverage stays within guardrails.

## Rollout

- Document the new requirement in `docs/design/ipfs-artifacts/README.md` and
  `docs/SNAPSHOTS.md`.
- Update `CHANGELOG.md` with the 2025-09-26 entry describing JetStream snapshot
  metadata streaming.
- Follow the RED → GREEN → REFACTOR cadence; integration/Grid verification
  resumes once workstation JetStream wiring stabilises.
