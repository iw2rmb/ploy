# roadmap-gitlab-node-bootstrap-04a – GitLab Credential Bootstrapper

- **Identifier**: `roadmap-gitlab-node-bootstrap-04a`
- [ ] **Status**: Planned (2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/design/control-plane/README.md`
- **Unblocks**:
  - `docs/tasks/roadmap/04b-gitlab-workflow-client.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 6

| Functional process                                | E | X | R | W | CFP |
| ------------------------------------------------- | - | - | - | - | --- |
| Beacon-issued mTLS handshake & bootstrap signer   | 1 | 1 | 1 | 1 | 4   |
| Node watcher & in-memory token refresh loop       | 1 | 0 | 1 | 0 | 2   |
| **TOTAL**                                         | 2 | 1 | 2 | 1 | 6   |

- Assumptions / notes: Control plane already publishes beacon-issued certificates; etcd hosts GitLab
  credential secrets populated via `ploy config gitlab`.

- **Why**
  - Nodes need ephemeral GitLab tokens fetched over mutual TLS so secrets never persist to disk.
  - Credential refresh must survive node restarts and rotate tokens before expiry to support safe MR
    automation.

- **How / Approach**
  - Implement a lightweight `internal/gitlab/signer` service embedding the stored PAT and minting
    short-lived access tokens.
  - Extend beacon bootstrap flow (`internal/node/bootstrap`) so nodes authenticate via mTLS, call the
    signer, and cache tokens in memory with jittered refresh.
  - Add etcd watch support for `config/gitlab.*` keys to trigger immediate token refresh when
    credentials rotate.

- **Changes Needed**
  - `internal/node/bootstrap/gitlab.go` (new) — handshake, token refresh loop, metrics.
  - `internal/gitlab/signer/` (new) — PAT-backed signer issuing scoped short-lived tokens.
  - `cmd/ploy/config_gitlab.go` — optionally emit signer endpoints in config responses.
  - `pkg/audit/logger.go` — record bootstrap attempts and refresh outcomes.

- **Definition of Done**
  - Nodes obtain GitLab tokens exclusively through the signer and hold credentials only in memory.
  - Token refresh executes automatically before expiry and after credential rotation events.
  - Bootstrap failures emit actionable logs/metrics for operators.

- **Tests To Add / Fix**
  - Unit: signer token issuance & scope validation.
  - Unit: node bootstrap refresh loop covering rotation and error backoff.
  - Integration: embedded signer mocked with mTLS handshake verifying startup and rotation path.

- **Dependencies & Blockers**
  - Requires completed GitLab config storage (`roadmap-gitlab-integration-04`).
  - Depends on beacon certificate distribution.

- **Verification Steps**
  - `go test ./internal/node/bootstrap/... ./internal/gitlab/signer/...`
  - Manual smoke: launch node with fake signer, inspect token refresh logs.

- **Changelog / Docs Impact**
  - Update `docs/v2/devops.md` and `docs/v2/mod.md` with bootstrap expectations and failure
    troubleshooting.

- **Notes**
  - Consider future reuse for other vendor integrations (GitHub, Bitbucket) once abstraction proves
    out.
