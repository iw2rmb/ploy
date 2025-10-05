# Snapshot Metadata Streams

- [x] Done (2025-09-26)

## Why / What For

Complete the snapshot toolkit’s contract by sending capture metadata to
JetStream so Grid consumers can hydrate snapshot fingerprints, rule counts, and
artifact CIDs without scraping CLI output.

## Required Changes

- Implement a JetStream-backed metadata publisher with schema-versioned
  envelopes for snapshot captures.
- Wire the CLI snapshot registry loader to select the JetStream publisher
  whenever discovery returns routes.
- Update docs (`docs/design/ipfs-artifacts/README.md`, `docs/SNAPSHOTS.md`) and
  changelog entries to reflect the discovery-driven behaviour.

## Definition of Done

- `ploy snapshot capture` publishes metadata to `ploy.artifact.<ticket>` when
  discovery returns JetStream routes, and surfaces errors when publishing fails.
- Offline runs (no discovery routes) continue to use the in-memory metadata
  publisher with identical output to today.
- Documentation references the JetStream metadata stream and no longer lists it
  as a TODO.

## Tests

- RED → GREEN: unit tests for the JetStream metadata publisher using an
  in-process JetStream server.
- CLI test covering registry configuration for discovery-backed routes vs
  in-memory fallback behaviour.
- Repository-wide `go test -cover ./...` to maintain ≥60% overall coverage and
  ≥90% in the snapshot package.
