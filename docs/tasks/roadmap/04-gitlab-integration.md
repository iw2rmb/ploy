# roadmap-gitlab-integration-04 – GitLab Integration & Credentials

- **Identifier**: `roadmap-gitlab-integration-04`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/design/control-plane/README.md`
  - `docs/v2/etcd.md`
  - `docs/v2/devops.md`
  - `docs/v2/cli.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05-cli-surface-refresh.md`
  - `docs/tasks/roadmap/06-api-surfaces.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 10

| Functional process                                  | E   | X   | R   | W   | CFP |
| --------------------------------------------------- | --- | --- | --- | --- | --- |
| etcd keyspace, RBAC policy, and watcher contracts   | 1   | 1   | 1   | 0   | 3   |
| mTLS bootstrap + ephemeral GitLab token minting     | 1   | 1   | 1   | 1   | 4   |
| CLI workflows for configure/validate/rotate secrets | 1   | 1   | 1   | 0   | 3   |
| **TOTAL**                                           | 3   | 3   | 3   | 1   | 10  |

- Assumptions / notes: GitLab PAT and deploy tokens remain available via Ops, and control plane mTLS
  automation (beacon-issued certificates) lands before this slice. Sandbox GitLab group exists for
  integration tests.

- **Why**
  - GitLab is the canonical source for Mods repositories and merge destinations in v2
    (`docs/v2/README.md`, `docs/v2/mod.md`).
  - Secrets must move from workstation env vars into etcd to align with the new control plane and to
    support multi-node clones without copying PATs manually.
  - Retiring Grid adapters requires native GitLab hooks, credential rotation, and audit coverage to
    satisfy security review.

- **How / Approach**
  - Define `config/gitlab.*` schema (token set, hostname allow-list, branch policies) plus RBAC
    metadata in etcd; expose typed accessors in a new `internal/config/gitlab` package.
  - Extend beacon-issued mTLS bootstrap so nodes request short-lived GitLab access tokens from a
    lightweight signer (wrapper around stored PAT) at startup; tokens live in memory only and refresh
    via watcher callbacks.
  - Replace legacy Grid GitLab clients with a native `internal/gitlab` module handling repository
    clone/push, merge request operations, and status updates through the official REST/GraphQL APIs.
  - Add CLI subcommands (`ploy config gitlab ...`, `ploy gitlab rotate`, `ploy gitlab status`) to
    create, validate, and rotate credentials without node restarts; extend audit logging to capture
    actor, scope, and expiry updates.
  - Update deployment scripts and docs so operators provision GitLab credentials during cluster
    bootstrap, including guidance for staging vs. production tokens.

- **Changes Needed**
  - `internal/config/gitlab/` (new) — typed etcd client, schema validation, RBAC helpers.
  - `internal/gitlab/` (new) — REST/GraphQL client abstraction with clone/push/MR helpers and
    token injection via interface.
  - `internal/node/bootstrap/gitlab.go` (new) — node-side credential hydrate + refresh loop bound to
    mTLS identity, writing temporary in-memory credential providers.
  - `internal/workflow/source/gitlab.go` — replace Grid adapter usage for fetch/push and MR status
    updates.
  - `cmd/ploy/config_gitlab.go`, `cmd/ploy/gitlab_rotate.go` (new) — CLI configure/validate/rotate
    flows with golden fixtures.
  - `pkg/audit/logger.go` — append GitLab credential events (create, rotate, revoke).
  - `scripts/bootstrap/cluster.sh` — prompt for GitLab token and project defaults during setup.
  - `docs/v2/devops.md`, `docs/v2/mod.md`, `docs/envs/README.md`, `docs/v2/cli.md` — document the new
    flows, environment fallbacks, and operational guidance.

- **Definition of Done**
  - Nodes fetch GitLab credentials from etcd via mTLS bootstrap, storing tokens only in memory and
    successfully cloning/pushing repos for Mods runs without Grid adapters.
  - `ploy config gitlab` manages credentials end-to-end (create, validate scopes, rotate) without
    requiring node restarts, and audit logs record each change with actor + expiry metadata.
  - Merge request flows (create/update/status) run solely through the new GitLab client abstraction,
    and docs explain scopes, rotation cadence, and incident response steps.

- **Tests To Add / Fix**
  - Unit: `internal/config/gitlab/config_test.go` covering schema validation, scope checks, RBAC.
  - Unit: `internal/gitlab/client_test.go` mocking REST/GraphQL interactions, covering clone/push/MR
    flows and token refresh hooks.
  - Integration: `tests/integration/gitlab/credentials_test.go` using GitLab sandbox API with mocked
    PAT signer to exercise clone + MR lifecycle using ephemeral tokens.
  - CLI: `cmd/ploy/gitlab_rotate_test.go` golden tests for happy path, insufficient scope, and
    rotation failure guidance.

- **Dependencies & Blockers**
  - Requires control plane mTLS and beacon certificate distribution (see
    `docs/design/control-plane/README.md`).
  - Needs GitLab sandbox group/project credentials plus infrastructure to mint short-lived tokens.
  - Audit logging pipeline must exist in `pkg/audit` before credential events are wired.

- **Verification Steps**
  - `go test ./internal/config/gitlab/... ./internal/gitlab/...`
  - `go test -tags integration ./tests/integration/gitlab/...`
  - `go test ./cmd/ploy -run TestGitlab*`
  - `make lint-md`

- **Changelog / Docs Impact**
  - Update `CHANGELOG.md` with credential rollout, tests executed, and doc sync summary.
  - Refresh `docs/v2/devops.md`, `docs/envs/README.md`, `docs/v2/mod.md`, and `docs/v2/cli.md` with
    setup instructions, rotation steps, and troubleshooting.

- **Next Steps**
  1. Land etcd schema + CLI configure/validate commands behind a feature flag (`PLOY_ENABLE_GITLAB_V2`).
  2. Integrate node bootstrap watcher and short-lived token signer, then remove Grid credential
     fallbacks once green.
  3. Capture audit events and document rotation + incident response runbooks before enabling by
     default.

- **Notes**
  - Evaluate GitLab deploy tokens vs. PATs for long-term automation depending on project policy.
  - Coordinate with security to rotate root PAT on a 30-day cadence and ensure sandbox tokens remain
    isolated from production repos.
