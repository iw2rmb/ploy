# roadmap-gitlab-credential-ops-04c – CLI Rotation & Audit Trail

- **Identifier**: `roadmap-gitlab-credential-ops-04c`
- [ ] **Status**: Planned (2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/tasks/roadmap/04a-gitlab-node-bootstrap.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05-cli-surface-refresh.md`
  - `docs/tasks/roadmap/08-deployment-bootstrap.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 5

| Functional process                                  | E | X | R | W | CFP |
| --------------------------------------------------- | - | - | - | - | --- |
| CLI rotate/status flows + validation messaging      | 1 | 1 | 1 | 0 | 3   |
| Audit & logging hooks for credential lifecycle      | 0 | 0 | 1 | 0 | 1   |
| Documentation + runbook updates                     | 0 | 0 | 1 | 0 | 1   |
| **TOTAL**                                           | 1 | 1 | 3 | 0 | 5   |

- Assumptions / notes: Base GitLab config commands (`show`, `set`) and etcd storage already landed.

- **Why**
  - Operators need supported workflows to rotate credentials without downtime and verify current
    scopes/state from the CLI.
  - Audit streams must capture credential changes for compliance.

- **How / Approach**
  - Extend `ploy config gitlab` with `rotate` and `status` subcommands that validate scopes, confirm
    PAT expiry, and emit actionable errors.
  - Hook into the audit logger to record create/rotate/revoke events with actor metadata.
  - Update docs/runbooks describing rotation cadence, incident response, and CLI usage.

- **Changes Needed**
  - `cmd/ploy/config_gitlab.go` & tests — add rotate/status flows, golden updates.
  - `pkg/audit/logger.go` — new events for credential lifecycle.
  - `docs/v2/devops.md`, `docs/envs/README.md`, `docs/v2/cli.md` — document rotation commands and
    audit expectations.

- **Definition of Done**
  - CLI can rotate GitLab credentials via etcd without manual node restarts and confirms new scopes
    to the operator.
  - `ploy config gitlab status` summarizes active tokens, expiry, and last rotation actor.
  - Audit log entries exist for create/rotate/revoke operations.

- **Tests To Add / Fix**
  - CLI golden tests covering rotate success/failure scenarios.
  - Unit tests for audit logger instrumentation.

- **Dependencies & Blockers**
  - Requires audit logger plumbing available.
  - Etcd credentials and signer service ready to accept rotated secrets.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestConfigGitlab*`
  - Simulated rotation in dev cluster verifying audit entries & node refresh.

- **Changelog / Docs Impact**
  - Append rotation guidance to `CHANGELOG.md` and update operational runbooks.

- **Notes**
  - Coordinate with security review to enforce rotation cadence (30 days) within CLI messaging.
