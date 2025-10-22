# roadmap-cli-surface-refresh-05d – Streaming & Observability Tooling

- **Identifier**: `roadmap-cli-surface-refresh-05d`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/tasks/roadmap/05c-cli-mods-artifacts.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05e-cli-operator-enablement.md`
  - `docs/tasks/roadmap/07b-job-log-archival.md`
  - `docs/tasks/roadmap/07c-job-observability-instrumentation.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                         | E | X | R | W | CFP |
|--------------------------------------------|---|---|---|---|-----|
| SSE client + retry logic                   | 1 | 1 | 1 | 0 | 3   |
| Observability command UX + documentation   | 0 | 0 | 1 | 0 | 1   |
| **TOTAL**                                  | 1 | 1 | 2 | 0 | 4   |

  - Assumptions / notes: Execution APIs expose SSE endpoints for job logs; CLI follows resilient streaming patterns discussed in recent Go SSE client guidance.

- **Why**
  - Operators need real-time job feedback aligned with the new execution APIs without relying on Grid dashboards.
  - Resilient streaming avoids manual polling and supports tooling that forwards logs to dashboards or files.

- **How / Approach**
  - Implement an SSE client with automatic reconnect, jittered backoff, and cursor resumption to handle transient errors.
  - Add `ploy logs` / `ploy watch` commands encapsulating job tailing, filters, output modes (plain, JSON), and piping support.
  - Expose observability hooks (e.g., job describe, log export) that integrate with future dashboards and job observability roadmap work.

- **Changes Needed**
  - `cmd/ploy/logs_command.go` (new) plus supporting streaming utilities.
  - `internal/workflow/runtime/local_client.go` or dedicated package for SSE connectors.
  - `docs/v2/logs.md`, `docs/v2/cli.md` – document streaming usage, retries, and troubleshooting.

- **Definition of Done**
  - CLI streams logs via SSE with retry/backoff and resumes from last event ID.
  - Commands provide actionable errors when log streams fail or job IDs are invalid.
  - Documentation covers streaming configuration, filters, and integration points.

- **Tests To Add / Fix**
  - Unit: SSE client tests with mocked server to cover reconnect paths.
  - Integration: run against local mock execution API verifying streaming, filtering, and cancellation.
  - Snapshot: help fixtures for new observability commands.

- **Dependencies & Blockers**
  - Requires Mods submission alignment so job metadata includes necessary identifiers and SHIFT gating context.
  - Needs stable execution API endpoints exposed by the control plane.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestLogs*`
  - Mock SSE harness demonstrating reconnect/resume behaviour.

- **Changelog / Docs Impact**
  - Document streaming additions, verification date, and evidence in `CHANGELOG.md`.
  - Update observability runbooks and dashboards with CLI streaming guidance.

- **Notes**
  - Consider fallback to buffered polling when SSE endpoints are unavailable; capture as a stretch goal or follow-up task.
