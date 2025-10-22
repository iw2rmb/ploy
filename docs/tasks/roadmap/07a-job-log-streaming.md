# roadmap-job-observability-07a – Job Log Streaming & Tails

- **Identifier**: `roadmap-job-observability-07a`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/design/mod-step-runtime/README.md`
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05d-cli-streaming-observability.md`
  - `docs/tasks/roadmap/08a-deployment-bootstrap-cli.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 5

| Functional process            | E | X | R | W | CFP |
| ----------------------------- | - | - | - | - | --- |
| Log streaming & SSE endpoints | 2 | 1 | 1 | 1 | 5   |
| **TOTAL**                     | 2 | 1 | 1 | 1 | 5   |

- Assumptions / notes: SSE-capable HTTP gateway already fronts the control plane; container runtime exposes stdout/stderr streams over gRPC.

- **Why**
  - Operators require real-time visibility into running Mod steps and SHIFT jobs without depending on Grid dashboards.
  - Newly provisioned clusters must offer job-tail experiences to support incident response and on-call triage.

- **How / Approach**
  - Capture container stdout/stderr via the step runtime and forward streams to the control plane with cursor metadata (event IDs, offsets).
  - Persist live log cursors in etcd so SSE handlers can resume clients and support pagination.
  - Implement SSE handlers with reconnect, back-pressure, and heartbeat semantics published under `/jobs/{id}/logs/stream`.
  - Surface stream state (active, complete, truncated) and include diagnostics (buffer pressures, dropped frames) in metadata.

- **Changes Needed**
  - `internal/controlplane/httpapi/logs.go` (new) – SSE streaming handlers, cursor management, throttling.
  - `internal/workflow/runtime/step/logs.go` – adapters that bridge container output into SSE buffers with replay caching.
  - `internal/workflow/logstream/` (new) – shared streaming utilities for cursors, heartbeats, and reconnect tokens.
  - `docs/v2/logs.md`, `docs/workflow/README.md` – document stream endpoints, reconnection guidance, and failure modes.

- **Definition of Done**
  - SSE log streaming works for active jobs with documented reconnect semantics and back-pressure handling.
  - CLI and dashboard clients can tail logs with resume support and detect stream completion.
  - Observability alerts capture stream failures or stalled writers for investigation.

- **Tests To Add / Fix**
  - Unit: SSE handler cursor math, reconnect token issuance, heartbeat timeouts.
  - Integration: run container step producing logs, validate live streaming via SSE, resume from saved cursor.
  - Load: simulate bursty log streams to confirm throttling/back-pressure behaviour.

- **Dependencies & Blockers**
  - Requires Mods step runtime pipeline updates to expose per-step stdout/stderr across the control plane.
  - Depends on IPFS artifact publisher for eventual archival handoff.

- **Verification Steps**
  - `go test ./internal/workflow/runtime/step -run TestLogs*`
  - `go test ./internal/controlplane/httpapi -run TestLogsStream*`
  - SSE smoke test script exercising reconnect + resume.

- **Changelog / Docs Impact**
  - Document SSE endpoints, reconnect semantics, and CLI tail usage in `docs/v2/logs.md`.
  - Capture verification evidence and dates in `CHANGELOG.md`.

- **Notes**
  - Evaluate streaming compression and tail window limits for long-lived steps.
  - Capture metrics for stream lag, retry counts, and client disconnect reasons for future dashboards.
