# roadmap-mod-step-runtime-03 – Node Artifact Publishing Integration

- **Identifier**: `roadmap-mod-step-runtime-03`
- [x] **Status**: Completed (2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
- **Unblocks**:
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 6

| Functional process                    | E   | X   | R   | W   | CFP |
| ------------------------------------- | --- | --- | --- | --- | --- |
| IPFS-backed artifact publisher wiring | 1   | 1   | 1   | 0   | 3   |
| CLI + runtime artifact exposure       | 1   | 1   | 0   | 0   | 2   |
| Verification + docs refresh           | 0   | 1   | 0   | 0   | 1   |
| **TOTAL**                             | 2   | 3   | 1   | 0   | 6   |

- Assumptions / notes: Assumes diff bundles are tarballs produced by the step
  runner and IPFS Cluster endpoints are available per
  `roadmap-ipfs-artifact-store-03`.

- **Why**
  - Enable Mods step runtime to publish captured diff/log tarballs through the
    new IPFS-based artifact store.
  - Surface artifact availability in CLI checkpoints so operators can inspect
    outputs without Grid dependencies.

- **How / Approach**
- Implement an artifact publisher module in `internal/workflow/runtime/step`
  that calls the IPFS Cluster client introduced in
  `roadmap-ipfs-artifact-store-03`.
- Update the local runtime client to attach published artifact CIDs to stage
  outcomes and expose retention metadata to the CLI.
- Refresh CLI fixture baselines and workflow docs to describe IPFS-backed
  artifact retrieval.

- **Changes Needed**
  - `internal/workflow/runtime/step/*` – wire artifact publisher implementations
    to diff/log tarball capture.
  - `internal/workflow/runtime/local_client.go` – persist published CIDs in
    stage outcomes.
  - `cmd/ploy/mod_run_*.go` & fixtures – display artifact references in outputs.
  - `docs/v2/job.md`, `docs/v2/mod.md`, `docs/workflow/README.md` – document
    artifact retrieval and inspection flows.
  - `CHANGELOG.md` – record verification evidence and doc sync.

- **Definition of Done**
- Diff and log tarballs produced by the step runner are published to IPFS
  Cluster via the runtime artifact publisher.
  - Stage outcomes include artifact CIDs and retention metadata surfaced by CLI
    inspection/status commands.
  - Documentation reflects IPFS-backed artifact access paths and verification
    steps are captured in `CHANGELOG.md`.

- **Tests To Add / Fix**
  - Unit: `internal/workflow/runtime/step/runner_test.go` to assert publisher
    invocation with tarball payloads.
  - Integration: `tests/integration/runtime/step_runner_test.go` (or follow-up
    IPFS-focused suite) validating publish + retention wiring using the Cluster
    client test doubles and the lab environment bootstrapped via
    `scripts/ipfs/bootstrap_lab_cluster.sh`.
  - Golden: `cmd/ploy/mod_run_output_test.go` capturing CLI artifact summaries.

- **Dependencies & Blockers**
  - Requires artifact store client from
    `docs/tasks/roadmap/03-ipfs-artifact-store.md`.
  - Depends on local runtime execution path from
    `docs/tasks/roadmap/02-mod-step-runtime.md`.
  - Requires access to the VPS lab IPFS Cluster provisioned via
    `scripts/ipfs/bootstrap_lab_cluster.sh` (targeting hosts from
    `docs/v2/vps-lab.md`) before integration suites execute; the script ensures
    Docker/compose are present on the lab hosts.

- **Verification Steps**
  - `scripts/ipfs/bootstrap_lab_cluster.sh` – provision the VPS lab IPFS
    Cluster over SSH before running integration suites; tear down after
    verification.
  - `go test ./internal/workflow/runtime/...`
  - `go test -tags integration ./tests/integration/runtime/...`
  - `go test ./cmd/ploy -run TestModRun*`
  - `make lint-md`

- **Changelog / Docs Impact**
  - Append dated entry describing artifact publisher integration, verification
    results, and updated docs.
  - Update workflow and v2 docs with IPFS retrieval guidance and note
    verification evidence.

- **Notes**
  - Coordinate with `roadmap-ipfs-artifact-store-03` to align retention policies
    and GC expectations.
  - Revisit SHIFT static check surfacing once IPFS-backed reports are available.
  - Completion recorded 2025-10-21 with verification in `CHANGELOG.md` (go test ./..., make lint-md).
