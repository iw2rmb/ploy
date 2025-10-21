# IPFS Cluster Artifact Store

## Why
- Ploy v2 publishes diffs, archives, logs, and OCI layers to IPFS Cluster for deterministic hydration across nodes (`docs/v2/README.md`).
- Centralizing artifact replication in IPFS replaces embedded IPFS nodes and removes Grid storage dependencies.

## Required Changes
- Stand up an IPFS Cluster client within each node and wire it to a shared pinset dedicated to Mods artifacts.
- Implement pin/unpin workflows with replication factors, monitoring health via Cluster metrics and alerting on drift, aligning with current IPFS Cluster operational guidance.citeturn0search8
- Encrypt or ACL-protect artifacts at rest where required, documenting trust bundles distributed through beacon mode.
- Update CLI commands for uploading/downloading artifacts to target IPFS Cluster endpoints only; remove Grid artifact code paths.

## Definition of Done
- Artifact publisher defaults to IPFS Cluster operations with configurable replication targets and verification.
- CLI users can fetch artifacts from any node, proving replication and hydration fidelity without Grid dependencies.
- Operational docs cover recoveries for pinset inconsistency and how to rotate trust bundles.

## Tests
- Unit tests for artifact publisher interfaces, including pinning retries and consistency checks.
- Smoke tests that upload artifacts, validate pin status across multiple nodes, then unpin and confirm garbage collection.
- CLI integration tests for artifact fetch commands ensuring checksum verification.
