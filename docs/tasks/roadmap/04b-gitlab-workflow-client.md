# roadmap-gitlab-workflow-client-04b – Native GitLab Runtime Integration

- **Identifier**: `roadmap-gitlab-workflow-client-04b`
- [ ] **Status**: Planned (2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/04a-gitlab-node-bootstrap.md`
  - `docs/tasks/roadmap/04-gitlab-integration.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05c-cli-mods-artifacts.md`
  - `docs/tasks/roadmap/07-job-observability.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 7

| Functional process                                    | E | X | R | W | CFP |
| ----------------------------------------------------- | - | - | - | - | --- |
| GitLab REST/GraphQL client abstraction + retries      | 1 | 1 | 1 | 0 | 3   |
| Workflow/runtime integration (clone/push/MR/status)   | 1 | 1 | 1 | 0 | 3   |
| Metrics & error surfacing for pipeline observability  | 0 | 0 | 1 | 0 | 1   |
| **TOTAL**                                             | 2 | 2 | 3 | 0 | 7   |

- Assumptions / notes: Node bootstrapper delivers in-memory tokens, and etcd hosts credential config.

- **Why**
  - Mods execution must detach from Grid hooks and use first-party GitLab clients for clone, push,
    and merge request operations.
  - Consistent error handling surfaced to CLI improves operator diagnostics.

- **How / Approach**
  - Create `internal/gitlab/client` wrapping REST+GraphQL endpoints (clone via HTTPS with token,
    branch creation, MR status updates) with typed responses and retry policy.
  - Update `internal/workflow/source/gitlab.go` and related runtime components to depend on the new
    client and signer token provider.
  - Emit structured metrics/logs for clone latency, MR creation, and failure reasons.

- **Changes Needed**
  - `internal/gitlab/client/` (new) — HTTP client, request signing, retries, metrics hooks.
  - `internal/workflow/source/gitlab.go` — replace Grid adapters, support scoped tokens.
  - `internal/workflow/runtime/local_client.go` — wire GitLab client into step execution pipeline.
  - `cmd/ploy/mod_run.go` — ensure CLI surfaces GitLab errors clearly.

- **Definition of Done**
  - Mods use the native GitLab client for repository operations and MR lifecycle actions.
  - Error messages identify GitLab error codes/scopes and bubble to CLI tests.
  - Metrics/logs record GitLab interactions for observability dashboards.

- **Tests To Add / Fix**
  - Unit: client request signing, retries, error decoding.
  - Unit: workflow source integration verifying clone/push path.
  - Integration: mock GitLab server tests covering MR lifecycle.

- **Dependencies & Blockers**
  - Requires bootstrapper delivering tokens (Task 04a).
  - GitLab sandbox endpoints for integration tests.

- **Verification Steps**
  - `go test ./internal/gitlab/... ./internal/workflow/source/...`
  - CLI smoke: `ploy mod run` against sandbox repo verifying branch + MR updates.

- **Changelog / Docs Impact**
  - Update `docs/v2/mod.md` and `docs/workflow/README.md` with the new GitLab flow.

- **Notes**
  - Capture instrumentation fields (project, branch, MR ID) for future job observability work.
