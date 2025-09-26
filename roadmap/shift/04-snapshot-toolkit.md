# Snapshot Toolkit
- [x] Done (2025-09-26)

## Why / What For
Give Ploy first-class commands to plan, capture, diff, and publish database snapshots without Nomad dependencies.

## Required Changes
- Implement `ploy snapshot plan` and `ploy snapshot capture` commands with strip/mask/synthetic rule engine.
- Wire artifact publishing to IPFS and metadata updates to JetStream.
- Provide local replay harness using lightweight containers that mirror Grid runtimes.

Status: Snapshot specs now live under `configs/snapshots/*.toml` with accompanying fixtures. The CLI loads the specs, summarises rules via `snapshot plan`, and executes the rule engine through `snapshot capture`, which applies strip/mask/synthetic transforms, generates deterministic fingerprints, and publishes metadata through the in-memory IPFS and JetStream publishers. Documentation describes the new commands and environment placeholders until real endpoints arrive. The replay harness currently uses deterministic fixture transforms; container-backed replays remain flagged for the JetStream/Grid integration slice.

## Definition of Done
- CLI can capture and replay snapshots for at least Postgres/MySQL test fixtures.
- JetStream carries snapshot metadata and IPFS hashes for reuse.
- Legacy SeaweedFS logic is deleted.

Outcome: `ploy snapshot capture --snapshot dev-db` runs against the included Postgres-style fixture, publishes a fake IPFS CID tagged with the deterministic fingerprint, and surfaces metadata ready for JetStream streams. SeaweedFS references remain absent, and the container replay hook is documented as a follow-up once JetStream connectivity is wired in.

## Tests
- Unit tests for rule engine transformations and diff summarisation.
- CLI tests covering snapshot command wiring and printing.
- Coverage metrics on the `internal/workflow/snapshots` package exceed 90%, keeping the workflow runner slices within guardrails.
