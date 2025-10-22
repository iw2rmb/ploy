# roadmap-deployment-bootstrap-08c – Operational Lifecycle & CI Validation

- **Identifier**: `roadmap-deployment-bootstrap-08c`
- [ ] **Status**: Planned (sized 2025-10-22)
- **Blocked by**:
  - `docs/tasks/roadmap/08a-deployment-bootstrap-cli.md`
  - `docs/tasks/roadmap/08b-deployment-services-automation.md`
  - `docs/tasks/roadmap/07b-job-log-archival.md`
- **Unblocks**:
  - `docs/tasks/roadmap/09-garbage-collection.md`
  - `docs/tasks/roadmap/10a-local-test-harness.md`
  - `docs/tasks/roadmap/10b-coverage-enforcement.md`
  - `docs/tasks/roadmap/10c-mods-timeouts-and-retries.md`
  - `docs/tasks/roadmap/10d-testing-docs-alignment.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 7

| Functional process                                   | E | X | R | W | CFP |
|------------------------------------------------------|---|---|---|---|-----|
| Rolling upgrade and node replacement automation      | 1 | 1 | 1 | 0 | 3   |
| Operational runbooks & certificate rotation guides   | 0 | 1 | 1 | 0 | 2   |
| CI smoke + regression pipelines for bootstrap flows  | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                            | 1 | 3 | 3 | 0 | 7   |

- Assumptions / notes:
  - Bootstrap workflows (08a) and service automation (08b) are functional and expose status APIs.
  - Log archival (07b) is available for post-mortem evidence in runbooks.

- **Why**
  - Operators require documented procedures and automation for scaling, certificate rotation, and node recovery once clusters are live.
  - CI must continuously validate bootstrap flows on clean infrastructure to prevent regressions.

- **How / Approach**
  - Implement rolling upgrade orchestration (node drain, redeploy, rejoin) with CLI hooks and safeguards.
  - Extend bootstrap controller with node replacement routines covering trust regeneration and service rebalancing.
  - Produce runbooks detailing scaling, rotation, failure recovery, and evidence capture using new observability/log tooling.
  - Introduce CI pipeline stage executing bootstrap smoke tests on ephemeral environments after `make build`.

- **Changes Needed**
  - `internal/bootstrap/lifecycle.go` (new) – rolling upgrade, node replacement, and certificate rotation helpers.
  - `cmd/ploy/bootstrap/lifecycle.go` – CLI commands (`upgrade`, `replace-node`, `rotate-certs`).
  - `tests/integration/bootstrap/` – smoke scenarios exercising upgrade and recovery flows.
  - `docs/runbooks/bootstrap/*.md` – operational procedures, completion checklists, and verification evidence expectations.
  - `docs/v2/devops.md`, `docs/workflow/README.md`, `docs/envs/README.md` – integrate lifecycle guidance and environment variables.

- **Definition of Done**
  - CLI supports rolling upgrades, node replacement, and certificate rotation with guardrails and audit logging.
  - Operational runbooks capture scaling, recovery, and verification steps with links to observability dashboards and log archives.
  - CI pipeline runs bootstrap smoke tests on every main merge, gating regressions.

- **Tests To Add / Fix**
  - Unit: lifecycle helper coverage for upgrade sequencing and trust rotation.
  - Integration: end-to-end VM/container suite validating upgrade, replacement, and failure handling.
  - CI: GitHub/GitLab pipeline wiring running bootstrap smoke workflow post-`make build`.

- **Dependencies & Blockers**
  - Relies on log archival (07b) to capture failure artefacts referenced in runbooks.
  - Needs service automation outputs (08b) for quorum-aware upgrade operations.

- **Verification Steps**
  - `go test ./internal/bootstrap -run TestLifecycle*`
  - `make build` followed by `make test` (ensuring integration suite covers lifecycle cases).
  - CI evidence appended to `CHANGELOG.md` with run IDs and timestamps.

- **Changelog / Docs Impact**
  - Update `CHANGELOG.md` with lifecycle automation release notes and CI verification runs.
  - Refresh operator runbooks, env references, and workflow documentation reflecting new procedures.

- **Notes**
  - Capture future work items for blue/green or canary bootstrap strategies if customer demand emerges.

