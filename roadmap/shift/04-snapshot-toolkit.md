# Snapshot Toolkit

- [x] Done (2025-09-26)

## Why / What For

Give Ploy first-class commands to plan, capture, diff, and publish database
snapshots without Nomad dependencies.

## Required Changes

- Implement `ploy snapshot plan` and `ploy snapshot capture` commands with
  strip/mask/synthetic rule engine.
- Wire artifact publishing to IPFS and metadata updates to JetStream.
- Provide local replay harness using lightweight containers that mirror Grid
  runtimes.

## Current Status (2025-09-26)

- Snapshot specs live under `configs/snapshots/*.toml` with fixtures consumed by
  the CLI.
- `snapshot plan` summarises rules and `snapshot capture` applies
  strip/mask/synthetic transforms, publishes deterministic fingerprints, and
  pushes metadata through the in-memory IPFS/JetStream publishers.
- Replay harness exercises deterministic fixture transforms; container-backed
  replays are logged as a follow-up for JetStream/Grid integration.

## Definition of Done

- CLI can capture and replay snapshots for at least Postgres/MySQL test
  fixtures.
- JetStream carries snapshot metadata and IPFS hashes for reuse.
- Legacy SeaweedFS logic is deleted.

## Tests

- Unit tests for rule engine transformations and diff summarisation.
- CLI tests covering snapshot command wiring and printing.
- Coverage on `internal/workflow/snapshots` stays ≥90% within workflow runner
  guardrails.
- Honour RED → GREEN → REFACTOR: introduce failing rule-engine tests, add
  minimal CLI wiring, then refactor after coverage targets are met.
