# roadmap-job-observability-07c – Job Metrics, Tracing & Dashboards

- **Identifier**: `roadmap-job-observability-07c`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/design/mod-step-runtime/README.md`
  - `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `docs/tasks/roadmap/05d-cli-streaming-observability.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08b-deployment-services-automation.md`
  - `docs/tasks/roadmap/08c-deployment-ops-ci.md`
  - `docs/tasks/roadmap/10a-local-test-harness.md`
  - `docs/tasks/roadmap/10b-coverage-enforcement.md`
  - `docs/tasks/roadmap/10c-mods-timeouts-and-retries.md`
  - `docs/tasks/roadmap/10d-testing-docs-alignment.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 3

| Functional process                       | E | X | R | W | CFP |
| ---------------------------------------- | - | - | - | - | --- |
| Metrics/tracing instrumentation & CLI UX | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                                | 1 | 1 | 1 | 0 | 3   |

- Assumptions / notes: Observability stack (Prometheus + Grafana) provisioned; tracing collector (OTel) reachable from workflow runner and control plane services.

- **Why**
  - Operators need dashboards highlighting job durations, exit codes, retry behaviour, and resource usage to triage failures quickly.
  - Tracing across workflow runner components surfaces bottlenecks and regression alerts as new Mod steps land.

- **How / Approach**
  - Instrument workflow runner, control plane handlers, and CLI commands with OpenTelemetry spans capturing job lifecycle events, tagging Mod metadata.
  - Emit metrics (counters, histograms) for job states, stream failures, retention expiry events, and CLI actions.
  - Publish Grafana dashboards and alert rules covering failure rates, backlog depth, and SLA thresholds.
  - Extend CLI with `ploy observe` commands to surface key metrics and dashboards, and to export traces for incident reviews.

- **Changes Needed**
  - `internal/workflow/observability/metrics.go` (new) – counters, histograms, structured labels.
  - `internal/workflow/observability/tracing.go` (new) – trace exporters, sampling configuration, span helpers.
  - `cmd/ploy/observe_*.go` (new) – CLI commands surfacing dashboards, metrics snapshots, and trace export utilities.
  - `docs/v2/logs.md`, `docs/v2/observability.md`, `docs/workflow/README.md` – document dashboards, alert thresholds, and CLI flows.

- **Definition of Done**
  - Metrics and traces emit to the observability stack with dashboards covering job success, failure, retry, and latency trends.
  - CLI exposes commands to inspect metrics/traces and link to dashboards.
  - Alerting rules fire for stalled jobs, elevated failure rates, or retention backlogs.

- **Tests To Add / Fix**
  - Unit: metrics registration, label validation, trace exporter configuration.
  - Integration: workflow runner execution path emitting spans and metrics captured by test collector.
  - CLI: snapshot or golden tests validating observability command outputs.

- **Dependencies & Blockers**
  - Requires stable streaming telemetry from task 07a and archival signals from task 07b to drive metrics.
  - Dependent on CLI streaming tooling (task 05d) for end-to-end observability flows.

- **Verification Steps**
  - `go test ./internal/workflow/observability -run TestMetrics*`
  - `go test ./cmd/ploy -run TestObserve*`
  - Manual: run sample workflow, confirm metrics/traces in Grafana/Tempo.

- **Changelog / Docs Impact**
  - Update observability documentation with new dashboards, alerts, and CLI flows.
  - Record verification commands plus timestamped evidence in `CHANGELOG.md`.

- **Notes**
  - Consider multi-tenant filtering for dashboards to isolate noisy neighbours.
  - Capture stretch goals for anomaly detection or SLO compliance reporting.
