# roadmap-control-plane-01 – Control Plane & Job Scheduling

- **Identifier**: `roadmap-control-plane-01`
- [x] **Status**: Completed (2025-10-21)
- **Blocked by**:
  - `docs/design/control-plane/README.md`
- **Unblocks**:
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 13

| Functional process       | E   | X   | R   | W   | CFP |
| ------------------------ | --- | --- | --- | --- | --- |
| Job submission API       | 2   | 1   | 1   | 1   | 5   |
| Job claim/lease workflow | 2   | 1   | 1   | 1   | 5   |
| Status query/reporting   | 1   | 0   | 1   | 0   | 2   |
| Runbook/GC hooks         | 0   | 1   | 0   | 0   | 1   |
| **TOTAL**                | 5   | 3   | 3   | 2   | 13  |

_Assumptions_: CFP counts treat each API path as one functional process; GC
hooks limited to metadata wiring for this slice.

- **Why**
  - Remove Grid dependency for Mods scheduling by introducing an etcd-backed
    control plane per `docs/design/control-plane/README.md`.

- **How / Approach**
  - Build scheduler + queue service using etcd transactions and leases.
  - Expose HTTP API endpoints for submission, claim, status, completion.
  - Provide retention and runbook tooling via explicit key prefixes.
  - Remove Grid leader-election references from runtime adapter registry.

- **Changes Needed**
  - `internal/controlplane/scheduler/*` – job service, optimistic txn logic,
    lease watchers.
  - `internal/controlplane/httpapi/*` – HTTP handlers for
    submission/claim/status.
  - `tests/integration/controlplane/*` – embedded etcd integration coverage.
  - `docs/design/control-plane/README.md` – keep design synced as implementation
    lands.
  - `docs/runbooks/control-plane/job-recovery.md` – operational recovery steps.
  - `docs/envs/README.md`, `docs/v2/README.md`, `docs/v2/queue.md`,
    `docs/v2/job.md` – align documentation with new scheduler behaviour.
  - `CHANGELOG.md` – record verification evidence and doc updates.

- **Definition of Done**
  - Control-plane service processes job submission, claim, status, completion
    purely via etcd keyspace described in design.
  - Single-claim guarantee enforced through transactions and leases.
  - Job status records persist through completion with retention + GC hooks.
  - Runbook documents stuck-job recovery using etcd keys (no Grid tools).
  - Scheduler packages free from Grid leader-election dependencies.

- **Tests To Add / Fix**
  - Unit: `internal/controlplane/scheduler/*_test.go` covering optimistic
    locking, lease renewal, retries, GC filters.
  - Integration: `tests/integration/controlplane/scheduler_test.go` spinning up
    embedded etcd with competing workers.
  - CLI smoke (follow-up once CLI integrates) – pending future slice.

- **Dependencies & Blockers**
  - Requires go.etcd.io/etcd/client/v3 module.
  - Coordination with future runtime slice for container execution.

- **Verification Steps**
  - 2025-10-21 — `go test ./internal/controlplane/... -cover`
  - 2025-10-21 —
    `go test -tags integration ./tests/integration/controlplane -cover`
  - 2025-10-21 — `staticcheck ./internal/controlplane/...`
  - 2025-10-21 — `make lint-md`

- **Changelog / Docs Impact**
  - Append dated entry summarising scheduler, docs, and verification commands.
  - Reference runbook creation and design verification.

- **Notes**
  - Capture etcd endpoints via env vars for local + CI usage
    (`PLOY_ETCD_ENDPOINTS` planned).
