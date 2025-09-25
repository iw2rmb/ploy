# NATS Self-Update Metrics Rollout

## Why / What For
- Lock in roadmap/nats Phase 3 (task 07) by instrumenting controller self-update flows with JetStream-native metrics.
- Surface bootstrap/queue health and executor progress so operators can validate the JetStream cutover without relying on Consul traces.
- Guard against duplicate submissions and ambiguous queue state by wiring HTTP responses to JetStream dedupe semantics.

## Current Constraints
- `api/selfupdate` publishes to JetStream but emits no Prometheus counters or histograms.
- Duplicate submissions bubble up as generic 500 errors despite `ErrDuplicateTask` dedupe hints.
- `initializeSelfUpdateHandler` provisions streams without observability or failure signalling.
- CLI tailers print status lines but cannot quantify lag between publication and consumption.

## Proposed Changes
1. **Metrics Surface**
   - Extend `api/metrics` with counters/histograms: `ploy_updates_js_bootstrap_total`, `ploy_updates_tasks_submitted_total`, `ploy_updates_executor_duration_seconds`, `ploy_updates_status_published_total`, `ploy_updates_redeliveries_total`, and a gauge `ploy_updates_status_consumer_lag_seconds`.
   - Provide helper methods so `selfupdate.Handler`, server bootstrap, and CLI commands can record metrics without hard dependencies.
2. **Handler Instrumentation**
   - Pass a metrics recorder into `selfupdate.Handler` and record task submissions, executor durations, status emissions, and redeliveries (labels: lane, strategy, result/phase).
   - Treat `ErrDuplicateTask` as a 409 conflict in HTTP handler, emitting a metrics sample with `result="duplicate"`.
3. **Bootstrap Observability**
   - Emit `ploy_updates_js_bootstrap_total{stream="tasks",status="success|error"}` and similar for status stream creation inside `initializeSelfUpdateHandler`.
4. **CLI Lag Gauge**
   - Track consumer lag (now minus event timestamp) when streaming status events; update `ploy_updates_status_consumer_lag_seconds{consumer="cli",lane}`.
5. **Roadmap Alignment**
   - Refresh `roadmap/nats/07-selfupdate-workqueue.md` to mark metrics deliverable complete and document new gauges.

## Definition of Done
- Unit tests cover metrics emission on submission, executor failure, and duplicate conflict path.
- Bootstrap metrics increment on success path; failure path covered via mock.
- CLI lag gauge updates per event.
- Roadmap task and CHANGELOG mention metrics visibility addition.

## Tests
- `api/selfupdate`: new tests asserting counters/histograms change after enqueue/execution/error.
- `api/server`: bootstrap test verifying counters after initializing handler with ephemeral JetStream.
- `internal/cli/updates`: unit test asserting lag gauge updates from consumed events (using fake JS message sequence).

## Rollout & Follow-ups
- Ship as part of controller release once Prometheus dashboards include the new metrics.
- Monitor for duplicate spikes; consider auto-surfacing deployment IDs in metrics descriptions if needed.
- Future work: integrate metrics with Nomad job manager for executor identity correlation.
