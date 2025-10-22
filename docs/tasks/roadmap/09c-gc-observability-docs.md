# roadmap-garbage-collection-09c – GC Observability & Safeguards

- **Identifier**: `roadmap-garbage-collection-09c`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/tasks/roadmap/09a-gc-retention-controllers.md`
  - `docs/tasks/roadmap/09b-gc-operator-cli.md`
- **Unblocks**:
  - `docs/tasks/roadmap/10a-local-test-harness.md`
  - `docs/tasks/roadmap/10b-coverage-enforcement.md`
  - `docs/tasks/roadmap/10c-mods-timeouts-and-retries.md`
  - `docs/tasks/roadmap/10d-testing-docs-alignment.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 7

| Functional process                             | E | X | R | W | CFP |
|------------------------------------------------|---|---|---|---|-----|
| Telemetry instrumentation & metrics export     | 1 | 1 | 1 | 0 | 3   |
| Dashboard & alert wiring in observability stack| 1 | 0 | 1 | 0 | 2   |
| Documentation & safeguard playbooks            | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                      | 2 | 2 | 3 | 0 | 7   |

- Assumptions / notes: Prometheus, Grafana, and log pipelines from roadmap task 07 are available; retention defaults defined in `docs/v2/gc.md` can be referenced directly; SOC and SRE contacts will review safeguard documentation before publication.

- **Why**
  - Observability is required to prove GC effectiveness, highlight reclaimed storage, and surface errors before they threaten availability.
  - Operators need documented safeguards (rate limits, dry-run paths, retention defaults) to follow during incidents and audits.

- **How / Approach**
  - Instrument GC controllers and CLI pathways to emit metrics (reclaimed bytes, candidate counts, error rates) and structured events consumed by the observability stack.
  - Build dashboards and alerts tracking GC cadence, failure spikes, and override usage, aligning naming with CLI output and audit logs.
  - Update retention documentation with per-environment defaults, safeguard checklists (dry-runs, rate limits, backoff), and communication templates for policy changes.
  - Integrate telemetry hooks with tracing/log aggregation to correlate GC runs, manual overrides, and downstream storage impacts.

- **Changes Needed**
  - `internal/observability/gc_metrics.go` (new) – counters/gauges/histograms plus exporters.
  - `internal/observability/dashboards/gc.json` (new) – Grafana dashboards and alert rules.
  - `internal/gc/controller/journal.go` – emit structured events compatible with logging pipeline.
  - `docs/v2/gc.md`, `docs/runbooks/gc.md`, `docs/envs/README.md` – retention defaults, safeguard procedures, environment variable expectations.
  - `docs/ops/incident-playbooks/gc.md` (new) – incident response steps for GC failures or overrides.

- **Definition of Done**
  - Metrics and logs capture every automated and manual GC run, including reclaimed storage, failures, and override metadata viewable in dashboards.
  - Alerts fire when GC lags behind target intervals, errors exceed thresholds, or manual overrides approach maximum allowed windows.
  - Documentation clearly lists retention defaults, rate limits, dry-run controls, and audit expectations per environment, and is linked from operator runbooks.

- **Tests To Add / Fix**
  - Unit: metrics exporter tests validating label cardinality and counter increments.
  - Integration: observability pipeline smoke tests ensuring GC events appear in Prometheus and logging systems.
  - Regression: docs link checker for new safeguard and playbook files.

- **Dependencies & Blockers**
  - Requires controller metrics surfaces (task 09a) and CLI override journaling (task 09b).
  - Depends on observability infrastructure bootstrapped by roadmap task 07.

- **Verification Steps**
  - `go test ./internal/observability -run TestGC*`
  - Deploy dashboard bundle to staging Grafana and validate metric presence with a seeded dry-run.
  - Run docs lint (`make docs-lint`) to ensure new references resolve.

- **Changelog / Docs Impact**
  - Add observability highlights and safeguard documentation updates to the release changelog.
  - Announce new dashboards and incident playbooks in operator comms.

- **Notes**
  - Coordinate with compliance to store audit logs per policy and to review override retention periods before production rollout.
