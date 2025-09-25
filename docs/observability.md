# JetStream Event Fabric

## Purpose
Document the platform-wide telemetry channels now backed by JetStream so operators and tooling authors know where to publish, consume, and troubleshoot build, allocation, and Mods events.

## Streams & Subjects
- **Build lifecycle**: `platform.builds` with subjects `build.status.<build_id>` retaining the most recent 72h of async build transitions. Messages carry the existing `buildStatus` JSON payload.
- **Allocation readiness**: `platform.allocs` with subjects `alloc.ready.<job_name>` reporting the first healthy allocation per job alongside task state summaries.
- **Mods telemetry**: `platform.mods` with subjects `mods.events.<execution_id>` mirroring runner events (`phase`, `step`, `level`, Nomad job metadata).

Each stream defaults to file storage, `MaxMsgsPerSubject=2048`, and replicas derived from `PLOY_EVENTS_*_REPLICAS`. Update `PLOY_EVENTS_MAX_AGE` to tune retention.

## Publishing Paths
- `api/server/build_async.go` publishes after every `writeStatus` call.
- `internal/orchestration/monitor.go` notifies when `WaitForHealthyAllocations` meets its quorum.
- `api/mods/handler.go` forwards runner events via the Mods API.

Publishers operate through `internal/events/fabric`, which also bootstraps the streams during controller startup. When JetStream is disabled or unreachable, the publisher falls back to a no-op.

## Consumption Patterns
- **CLI / automation**: Use durable pull consumers with modest `Fetch` sizes. The controller’s SSE endpoints (`/v1/apps/:app/builds/:id/events`) now bind a pull consumer internally and fall back to file polling if JetStream is unavailable.
- **Example durable**: `nats consumer add platform.builds build-events-cli --filter="build.status.<id>" --deliver=all --ack explicit --max-ack-pending=32`.
- **Streaming tips**: Track lag via Prometheus (`ploy_updates_status_consumer_lag_seconds` covers CLI consumers; add matching metrics for builds/allocs in follow-up if needed).

## Configuration
Set the following environment variables on the controller:

| Variable | Description | Default |
| --- | --- | --- |
| `PLOY_EVENTS_JETSTREAM_URL` | JetStream endpoint for the event fabric | inherits `PLOY_JETSTREAM_URL` |
| `PLOY_EVENTS_BUILD_STREAM` | Stream name for build status events | `platform.builds` |
| `PLOY_EVENTS_BUILD_SUBJECT` | Subject prefix for build events | `build.status` |
| `PLOY_EVENTS_ALLOC_STREAM` | Stream name for allocation events | `platform.allocs` |
| `PLOY_EVENTS_ALLOC_SUBJECT` | Subject prefix for allocation events | `alloc.ready` |
| `PLOY_EVENTS_MODS_STREAM` | Stream name for Mods telemetry | `platform.mods` |
| `PLOY_EVENTS_MODS_SUBJECT` | Subject prefix for Mods telemetry | `mods.events` |
| `PLOY_EVENTS_MAX_AGE` | Max retention per stream | `72h` |

Credentials follow the existing JetStream conventions (`PLOY_EVENTS_JETSTREAM_CREDS`, `PLOY_EVENTS_JETSTREAM_USER`, `PLOY_EVENTS_JETSTREAM_PASSWORD`).

## Troubleshooting
1. Verify stream provisioning: `nats stream info platform.builds`.
2. Check consumer state: `nats consumer info platform.builds build-events-sse-<id>`.
3. Inspect lag: `nats consumer info platform.allocs alloc-ready-cli --json | jq '.num_pending'`.
4. Fall back to legacy polling by disabling the event fabric (`PLOY_EVENTS_JETSTREAM_ENABLED=false`) if JetStream is undergoing maintenance; publishers revert to no-ops and SSE endpoints continue polling status files/Nomad.

## Related Documentation
- `docs/runbooks/jetstream.md` — operational procedures for the JetStream control plane.
- `roadmap/nats/09-event-fabric.md` — design and rollout context for this migration.
- `CHANGELOG.md` — release notes covering the event fabric consolidation.
