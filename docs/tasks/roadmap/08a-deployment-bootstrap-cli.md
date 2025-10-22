# roadmap-deployment-bootstrap-08a – Cluster Bootstrap CLI

- **Identifier**: `roadmap-deployment-bootstrap-08a`
- [ ] **Status**: Planned (sized 2025-10-22)
- **Blocked by**:
  - `docs/tasks/roadmap/04c-gitlab-credential-ops.md`
  - `docs/tasks/roadmap/05e-cli-operator-enablement.md`
  - `docs/tasks/roadmap/07-job-observability.md`
  - `docs/tasks/roadmap/07a-job-log-streaming.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08b-deployment-services-automation.md`
  - `docs/tasks/roadmap/08c-deployment-ops-ci.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 8

| Functional process                             | E | X | R | W | CFP |
|-----------------------------------------------|---|---|---|---|-----|
| CLI bootstrap command & UX scaffolding         | 1 | 1 | 1 | 0 | 3   |
| Trust bundle generation & worker registration  | 1 | 1 | 1 | 0 | 3   |
| Pre-flight validation & prerequisite capture   | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                      | 2 | 3 | 3 | 0 | 8   |

- Assumptions / notes:
  - Base CLI packaging, auth flows, and operator enablement from roadmap slice 05e exist.
  - Log streaming (07a) is available for bootstrap diagnostics.

- **Why**
  - Operators need a single CLI workflow to bootstrap a Ploy v2 cluster (beacon, etcd, IPFS Cluster, worker nodes) without Grid dependencies.
  - Automation must provision trust bundles and register workers consistently across clean Linux hosts as outlined in `docs/v2/devops.md`.

- **How / Approach**
  - Introduce `ploy bootstrap cluster` with sub-steps for beacon provisioning, trust bundle creation, and worker registration.
  - Implement a reusable bootstrap controller in `internal/bootstrap/cluster` orchestrating host discovery, SSH orchestration, and state tracking.
  - Integrate pre-flight validation covering OS, Docker, virtualization, disk space, and SSH prerequisites; surface actionable remediation guidance.
  - Emit structured progress events for observability and audit trails.

- **Changes Needed**
  - `cmd/ploy/bootstrap/*.go` – new CLI entrypoints, flag parsing, and progress UX.
  - `internal/bootstrap/cluster/` (new) – orchestration, inventory management, state persistence.
  - `internal/bootstrap/preflight.go` (new) – prerequisite scanning, remediation hints, test doubles.
  - `docs/v2/devops.md`, `docs/workflow/README.md` – describe bootstrap workflow, inputs, and outputs.
  - `docs/envs/README.md` – reference required environment variables and secrets.

- **Definition of Done**
  - `ploy bootstrap cluster` provisions beacon, trust bundle, and worker nodes on clean hosts with idempotent retries.
  - CLI surfaces pre-flight failures before any destructive actions and records results in bootstrap reports.
  - Bootstrap reports capture node inventory, generated certificates, and registration confirmations for audit.

- **Tests To Add / Fix**
  - Unit: pre-flight check coverage, trust bundle generation, worker registration request building.
  - Integration: disposable VM/container suite executing the bootstrap workflow end-to-end.
  - CLI golden tests for progress output and failure messaging.

- **Dependencies & Blockers**
  - Requires credential operations (04c) to supply GitLab secrets during bootstrap.
  - Observability slices (07, 07a) must provide log streaming for debugging.

- **Verification Steps**
  - `go test ./cmd/ploy/bootstrap -run TestCluster*`
  - `go test ./internal/bootstrap/...`
  - Grid/VPS: run bootstrap integration suite against clean sandbox inventory and attach logs.

- **Changelog / Docs Impact**
  - Add dated entry to `CHANGELOG.md` documenting the bootstrap command, verification runs, and evidence.
  - Update operator runbooks referencing Grid bootstrap flows with the CLI-driven sequence.

- **Notes**
  - Capture follow-up tasks for multi-region bootstrap or HA beacon scaling if identified during implementation.

