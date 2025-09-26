# Snapshot Metadata Streams
- [x] Done (2025-09-26)

## Why / What For
Complete the snapshot toolkit’s contract by sending capture metadata to JetStream so Grid consumers can hydrate snapshot fingerprints, rule counts, and artifact CIDs without scraping CLI output.

## Required Changes
- Implement a JetStream-backed metadata publisher with schema-versioned envelopes for snapshot captures.
- Wire the CLI snapshot registry loader to select the JetStream publisher whenever ``JETSTREAM_URL`` is configured.
- Update docs (`docs/design/ipfs-artifacts/README.md`, `docs/SNAPSHOTS.md`) and changelog entries to reflect the new behaviour and environment requirement.

## Definition of Done
- `ploy snapshot capture` publishes metadata to `ploy.artifact.<ticket>` when ``JETSTREAM_URL`` is set, and surfaces errors when publishing fails.
- Offline runs (no ``JETSTREAM_URL``) continue to use the in-memory metadata publisher with identical output to today.
- Documentation references the JetStream metadata stream and no longer lists it as a TODO.

## Tests
- RED → GREEN: unit tests for the JetStream metadata publisher using an in-process JetStream server.
- CLI test covering registry configuration for ``JETSTREAM_URL`` vs fallback behaviour.
- Repository-wide `go test -cover ./...` to maintain ≥60% overall coverage and ≥90% in the snapshot package.
