# roadmap-ipfs-artifact-store-03 – IPFS Cluster Artifact Store

- **Identifier**: `roadmap-ipfs-artifact-store-03`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/design/mod-step-runtime/README.md`
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
- **Unblocks**:
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 9

| Functional process                         | E | X | R | W | CFP |
|--------------------------------------------|---|---|---|---|-----|
| Cluster client wiring & auth               | 1 | 1 | 1 | 0 | 3   |
| Pinset management & replication monitoring | 1 | 1 | 1 | 1 | 4   |
| CLI integration for uploads/downloads      | 1 | 0 | 1 | 0 | 2   |
| **TOTAL**                                  | 3 | 2 | 3 | 1 | 9   |

- Assumptions / notes: CFP assumes IPFS Cluster endpoints already
  provisioned; scope limited to Mods artifacts and workstation validation.

- **Why**
  - Ploy v2 publishes diffs, archives, logs, and OCI layers to IPFS Cluster for
    deterministic hydration across nodes (`docs/v2/README.md`).
  - Centralising artifact replication in IPFS replaces embedded IPFS nodes and
    removes Grid storage dependencies documented in `docs/v2/mod.md`.

- **How / Approach**
  - Embed an IPFS Cluster client within each node, configuring shared pinsets
    dedicated to Mods artifacts with workspace-supplied credentials.
  - Implement pin/unpin workflows with replication factors, health metrics, and
    alerting aligned with current IPFS Cluster operational
    guidance.citeturn0search8
  - Encrypt or ACL-protect pinned artifacts where required, documenting trust
    bundle distribution through beacon mode.
  - Update CLI artifact commands to target Cluster endpoints exclusively,
    removing Grid artifact code paths while preserving checksum verification.

- **Changes Needed**
  - `internal/workflow/runtime/step/*` – call artifact publisher interface that
    wraps IPFS Cluster.
  - `internal/workflow/artifacts/*` (new) – client abstraction for pin/unpin,
    health polling, retries.
  - `cmd/ploy/artifact_*.go` – CLI upload/download wiring, status output, error
    handling.
  - `configs/` and `docs/envs/README.md` – document Cluster endpoints,
    credentials, replication knobs.
  - `docs/v2/ipfs.md`, `docs/workflow/README.md` – reflect operational flows and
    retention policies.

- **Definition of Done**
  - Artifact publisher defaults to IPFS Cluster with configurable replication
    targets and verification hooks.
  - CLI users can fetch artifacts from any node, demonstrating replication and
    hydration fidelity without Grid dependencies.
  - Operational docs cover recovery for pinset inconsistency and trust bundle
    rotation.

- **Tests To Add / Fix**
  - Unit: `internal/workflow/artifacts/*_test.go` covering client retries,
    consistency checks, ACL enforcement.
  - Integration: `tests/integration/artifacts/ipfs_cluster_test.go` uploading
    artifacts, verifying replication across multiple nodes, unpinning, and
    confirming garbage collection.
  - CLI: `cmd/ploy/artifact_command_test.go` verifying upload/download flows and
    checksum validation.

- **Dependencies & Blockers**
  - Requires Mods step runtime pipeline to emit diff/log artifacts (`docs/tasks/roadmap/02-mod-step-runtime.md`).
  - Needs access to IPFS Cluster endpoints and credentials distributed via beacon.

- **Verification Steps**
  - `go test ./internal/workflow/artifacts/...`
  - `go test -tags integration ./tests/integration/artifacts/...`
  - `make lint-md`

- **Changelog / Docs Impact**
  - Append dated entry summarising IPFS integration, verification commands, and documentation refreshes.
  - Update `docs/v2/ipfs.md`, `docs/envs/README.md`, and runbooks with new operational guidance.

- **Notes**
  - Evaluate Cluster sharding/replication factors for heterogeneous node capacity.
  - Plan follow-up to integrate IPFS health metrics with job observability dashboards.
