# roadmap-deployment-bootstrap-08b – Cluster Services Automation

- **Identifier**: `roadmap-deployment-bootstrap-08b`
- [ ] **Status**: Planned (sized 2025-10-22)
- **Blocked by**:
  - `docs/tasks/roadmap/08a-deployment-bootstrap-cli.md`
  - `docs/tasks/roadmap/07c-job-observability-instrumentation.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08c-deployment-ops-ci.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 8

| Functional process                                  | E | X | R | W | CFP |
|-----------------------------------------------------|---|---|---|---|-----|
| IPFS Cluster automation and health lifecycle        | 1 | 1 | 1 | 0 | 3   |
| etcd bootstrapping, trust, and backup provisioning  | 1 | 1 | 1 | 0 | 3   |
| Alert wiring, telemetry validation, and run hooks   | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                           | 2 | 3 | 3 | 0 | 8   |

- Assumptions / notes:
  - Cluster bootstrap CLI (08a) already provisions node inventory and trust bundles.
  - Observability instrumentation (07c) exports metrics/alerts for service health checks.

- **Why**
  - Core dependencies (IPFS Cluster, etcd) must be provisioned and configured automatically to eliminate manual Grid-era runbooks.
  - Health verification and alerting ensure freshly bootstrapped clusters are production ready before workload onboarding.

- **How / Approach**
  - Extend bootstrap controller with service roles managing IPFS peers, replication factors, and pinning policies.
  - Automate etcd cluster creation, TLS trust distribution, snapshot scheduling, and quorum validation.
  - Emit health probes and alert hooks wiring into the observability stack; expose CLI commands for status and remediation guidance.
  - Bake configuration artifacts (systemd units, kube manifests) into bootstrap templates stored in repo-controlled bundles.

- **Changes Needed**
  - `internal/bootstrap/services/ipfs.go` (new) – peer automation, replication policy enforcement, health diagnostics.
  - `internal/bootstrap/services/etcd.go` (new) – cluster formation, snapshot wiring, disaster recovery hooks.
  - `internal/bootstrap/services/status.go` (new) – consolidated health checks exposed to CLI and CI.
  - `cmd/ploy/bootstrap/services.go` – CLI surfaces for service status, repair, and validation.
  - `docs/v2/devops.md`, `docs/workflow/README.md`, `docs/runbooks/bootstrap/` – document automated service configuration and fallback paths.

- **Definition of Done**
  - Bootstrap automation configures IPFS Cluster and etcd with replication, trust, and alerting defaults aligned with Ploy v2 requirements.
  - CLI exposes `ploy bootstrap services --status|--repair` to verify health and remediate drift.
  - Health telemetry feeds observability dashboards with actionable alerts for quorum loss, pinning lag, or snapshot failures.

- **Tests To Add / Fix**
  - Unit: IPFS peer assignment logic, etcd snapshot scheduling, alert hook validation.
  - Integration: ephemeral environment verifying IPFS pin replication and etcd quorum under bootstrap automation.
  - Smoke: CLI checks for service health executed in CI after bootstrap workflow.

- **Dependencies & Blockers**
  - Requires observability dashboards/metrics from task 07c to attach alerts.
  - Depends on cluster identity/trust artifacts emitted by task 08a.

- **Verification Steps**
  - `go test ./internal/bootstrap/services -run Test*`
  - Bootstrap integration suite validating IPFS/etcd health across three-node clusters.
  - Observability smoke run confirming alerts fire when a peer is intentionally isolated.

- **Changelog / Docs Impact**
  - Document automated service configuration, alert defaults, and recovery steps in `docs/v2/devops.md` and runbooks.
  - Record verification evidence with timestamps in `CHANGELOG.md`.

- **Notes**
  - Track follow-up work for multi-region replication or external etcd storage if needed by customers.

