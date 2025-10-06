# Event Fabric Consolidation

> **Status (2025-09-25):** Completed — JetStream streams now back build
> lifecycle, Nomad readiness, and Mods telemetry with CLI/SSE consumption via
> pull consumers.

## What to Achieve

Stream platform events—build lifecycle, Nomad allocation readiness, Mods
telemetry—into JetStream so CLIs and services consume them via pull consumers
instead of bespoke HTTP polling.

## Why It Matters

A unified event fabric simplifies observability, reduces load on the controller,
and enables future automations to react to platform signals in real time.

## Where Changes Will Affect

- `internal/orchestration/` (log streamer, monitor) – publish readiness events.
- `internal/mods/events_emit.go`, controller reporters – redirect output to
  JetStream subjects.
- CLI/UI clients – subscribe to JetStream subjects for live updates.
- Documentation (`docs/observability.md`, CLI docs) – explain subscription
  patterns and troubleshooting.

## How to Implement

1. **Streams + Subjects**: Provisioned idempotent JetStream streams
   `platform.builds`, `platform.allocs`, and `platform.mods` with subjects
   `build.status.*`, `alloc.ready.*`, and `mods.events.*`, enforcing
   `MaxAge=72h`, replicas from env, and limits policy retention.
2. **Publishers**: Controller now marshals async build status (`writeStatus`),
   health monitor readiness signals, and Mods API events into the fabric via a
   shared publisher (`internal/events/fabric`). Existing file/SSE paths remain
   as fallbacks during rollout.
3. **Consumers**: CLI/SSE endpoints bind durable pull consumers with bounded
   `Fetch` loops (`build-events-sse-<id>`) so backpressure is honoured; HTTP
   clients still work without JetStream via fallback polling.
4. **Docs & Config**: Added `JetStreamEvents` controller config, documented env
   vars, and noted the change in `docs/OBSERVABILITY.md` plus `CHANGELOG.md`.
5. **Validation**: Unit tests assert publishers fire (`api/server`,
   `internal/orchestration`, `api/mods`), and package-level tests cover stream
   bootstrap; runbooks will reference the new streams in the next release note
   refresh.

## Expected Outcome

Platform telemetry flows through JetStream, enabling multiple subscribers to
consume events reliably without controller polling; legacy HTTP/SSE clients
degrade gracefully via bridged pull consumers.

## Tests

- **Unit**: `api/server/build_async_publish_test.go`,
  `internal/orchestration/monitor_wait_test.go`, and `api/mods/events_test.go`
  verify publishers fire JetStream notifications.
- **Integration**: Covered via controller startup ensuring streams bootstrap
  with live JetStream; SSE bridge fetch verified manually against dev cluster.
- **E2E**: Pending automation—manual verification performed by tailing JetStream
  subjects during a staging deploy; add scripted coverage in Phase 10.
