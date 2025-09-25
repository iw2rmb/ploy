# Snapshot Toolkit
- [ ] Pending

## Why / What For
Give Ploy first-class commands to plan, capture, diff, and publish database snapshots without Nomad dependencies.

## Required Changes
- Implement `ploy snapshot plan` and `ploy snapshot capture` commands with strip/mask/synthetic rule engine.
- Wire artifact publishing to IPFS and metadata updates to JetStream.
- Provide local replay harness using lightweight containers that mirror Grid runtimes.

## Definition of Done
- CLI can capture and replay snapshots for at least Postgres/MySQL test fixtures.
- JetStream carries snapshot metadata and IPFS hashes for reuse.
- Legacy SeaweedFS logic is deleted.

## Tests
- Unit tests for rule engine transformations and diff summarisation.
- Integration tests using containerised databases to validate capture/replay flow.
- Coverage metrics meet 90% for snapshot packages due to critical path classification.
