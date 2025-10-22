# roadmap-gitlab-docs-and-tests-04d – Documentation & Integration Coverage

- **Identifier**: `roadmap-gitlab-docs-and-tests-04d`
- [ ] **Status**: Planned (2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/04b-gitlab-workflow-client.md`
  - `docs/tasks/roadmap/04c-gitlab-credential-ops.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05e-cli-operator-enablement.md`
  - `docs/tasks/roadmap/10a-local-test-harness.md`
  - `docs/tasks/roadmap/10b-coverage-enforcement.md`
  - `docs/tasks/roadmap/10c-mods-timeouts-and-retries.md`
  - `docs/tasks/roadmap/10d-testing-docs-alignment.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                                  | E | X | R | W | CFP |
| --------------------------------------------------- | - | - | - | - | --- |
| Sandbox integration tests (clone, MR, rotation)     | 1 | 0 | 1 | 0 | 2   |
| Documentation sync across v2 guides & runbooks      | 0 | 0 | 1 | 0 | 1   |
| Incident response & troubleshooting playbooks       | 0 | 0 | 1 | 0 | 1   |
| **TOTAL**                                           | 1 | 0 | 3 | 0 | 4   |

- Assumptions / notes: Runtime client and CLI rotation already landed; sandbox GitLab project
  available for automated testing.

- **Why**
  - We must prove GitLab flows end-to-end (bootstrap → clone → MR → rotation) and document how
    operators recover from credential issues.

- **How / Approach**
  - Add integration suites under `tests/integration/gitlab` exercising token bootstrap, clone,
    MR creation/update, and rotation error handling using sandbox tokens.
  - Refresh `docs/v2/devops.md`, `docs/v2/mod.md`, `docs/v2/cli.md`, and `docs/workflow/README.md`
    to reflect final flows and troubleshooting.
  - Author incident response playbooks covering leaked tokens, signer failure, and GitLab outages.

- **Changes Needed**
  - `tests/integration/gitlab/` — new suites with sandbox fixtures and rotation scenarios.
  - `docs/v2/*` and runbooks — updates with screenshots/examples of new commands and recovery steps.
  - `CHANGELOG.md` — entry summarising verification results.

- **Definition of Done**
  - Integration tests pass locally and in CI, validating clone/MR/rotation flows.
  - Documentation comprehensively covers setup, rotation, troubleshooting, and incident response.

- **Tests To Add / Fix**
  - `go test -tags integration ./tests/integration/gitlab/...`

- **Dependencies & Blockers**
  - Sandbox GitLab availability and seeded projects.
  - Audit logger instrumentation from Task 04c for doc references.

- **Verification Steps**
  - Run integration suites and capture results in `CHANGELOG.md`.

- **Changelog / Docs Impact**
  - Update docs and runbooks listed above.

- **Notes**
  - Coordinate with security team to align incident response docs with corporate policy.
