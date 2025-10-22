# roadmap-job-observability-07 – Job Observability & Logs

- **Identifier**: `roadmap-job-observability-07`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/design/mod-step-runtime/README.md`
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08-deployment-bootstrap.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 12

| Functional process                       | E   | X   | R   | W   | CFP |
| ---------------------------------------- | --- | --- | --- | --- | --- |
| Log streaming & SSE endpoints            | 2   | 1   | 1   | 1   | 5   |
| Log archival & retention policies        | 1   | 1   | 1   | 1   | 4   |
| Metrics/tracing instrumentation & CLI UX | 1   | 1   | 1   | 0   | 3   |
| **TOTAL**                                | 4   | 3   | 3   | 2   | 12  |

- Assumptions / notes: CFP assumes IPFS artifact publisher available for
  archival and observability stack (Prometheus + Grafana) already provisioned.

- **Why**
  - Every Mod step and SHIFT run must persist stdout/stderr, expose SSE tails,
    and keep containers available for inspection (`docs/v2/job.md`,
    `docs/v2/logs.md`).
  - Operators need first-class observability detached from Grid logging
    pipelines to debug workstation runs.

- **How / Approach**
  - Build a log aggregation pipeline streaming container output to etcd metadata
    and IPFS payload storage, exposing SSE endpoints through the control plane.
  - Implement retention policies and garbage collection hooks aligned with
    `docs/v2/gc.md`, configurable per Mod or organisation.
  - Add tracing and metrics instrumentation (timings, exit codes, retry counts)
    exported to the observability stack with dashboards for quick triage.
  - Provide CLI tooling for real-time tails, historical log pulls, and
    inspection of retained containers, aligning with retention guarantees.

- **Changes Needed**
  - `internal/controlplane/httpapi/logs.go` (new) – SSE streaming handlers,
    pagination, reconnect semantics.
  - `internal/workflow/runtime/step/logs.go` – log capture adapters writing to
    IPFS and etcd metadata.
  - `internal/workflow/observability/*` (new) – metrics counters, histograms,
    trace exporters.
  - `cmd/ploy/logs_*.go` – CLI tails, log fetch commands, inspection wiring.
  - `docs/v2/logs.md`, `docs/v2/gc.md`, `docs/workflow/README.md` – document log
    retention and inspection workflows.

- **Definition of Done**
  - SSE log streaming works for active jobs with documented reconnect semantics
    and back-pressure handling.
  - Historical logs and artifacts can be fetched after job completion,
    respecting retention windows and ACLs.
  - Observability dashboards surface run-level metrics and alert on failures or
    stalled jobs.

- **Tests To Add / Fix**
  - Unit: log persistence adapters, SSE streaming handlers, retention policy
    evaluation.
  - Integration: capture live container output, verify IPFS archival, replay
    logs via CLI.
  - Load: simulate high-volume log streams to validate back-pressure and
    resource usage.

- **Dependencies & Blockers**
  - Requires Mods step runtime pipeline (task 02) and IPFS artifact publisher
    (task 03).
  - Depends on control plane job metadata to include retention + container
    identifiers.

- **Verification Steps**
  - `go test ./internal/workflow/runtime/step -run TestLogs*`
  - `go test ./internal/controlplane/httpapi -run TestLogs*`
  - `go test ./cmd/ploy -run TestLogs*`

- **Changelog / Docs Impact**
  - Document SSE endpoints, retention policies, and CLI inspection flows in
    `docs/v2/logs.md` and runbooks.
  - Record verification evidence in `CHANGELOG.md` with commands and dates.

- **Notes**
  - Evaluate streaming compression and tail window limits for long-lived steps.
  - Consider integrating structured log parsing for SHIFT findings as part of
    the same pipeline.
