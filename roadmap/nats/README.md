# NATS JetStream Migration Guide

## Purpose
- Replace Consul KV as the coordination and config store for control-plane services.
- Unlock push-based workflows for platform events (env vars, routing, certificates, self-updates).
- Reduce moving parts by consolidating ephemeral locks, state snapshots, and telemetry onto JetStream primitives.

## Narrative Summary
Consul remains the only strongly stateful dependency after SeaweedFS; its KV usage spans environment variables, DNS/certificate metadata, Mods knowledge-base locks, and controller self-updates. The plan below phases JetStream in as the primary coordination plane. JetStream Key-Value buckets provide low-latency state with revision semantics, while Streams/Durable Consumers enable fan-out notifications that today require polling or ad-hoc HTTP calls. Service discovery (Consul catalog + Traefik) stays in place for now; the scope here is the KV layer and dependent workflows.

## Key Files
- `api/consul_envstore/store.go#L135` – Consul-backed app environment store and cache.
- `internal/orchestration/kv.go#L10` – Thin Consul KV adapter used throughout orchestration helpers.
- `internal/routing/README.md` – JetStream routing store, sync helpers, and operational guidance.
- `api/acme/storage.go#L248` – Certificate metadata storage in Consul KV.
- `api/certificates/manager.go#L296` – App certificate registry persisted via Consul KV.
- `api/dns/handler.go#L88` – DNS configuration snapshotting into Consul KV.
- `api/selfupdate/executor.go#L15` – Controller self-update coordination and KV-backed session status.
- `internal/mods/kb_locks.go#L31` – Mods Knowledge Base distributed locks built on Consul sessions.

## Current Consul KV Footprint
- **Environment variables** (`api/consul_envstore`, `api/server/initializers_infra.go`) cache JSON blobs per app with optional batching and invalidation.
- **Routing + DNS metadata**: JetStream object storage now owns route maps (see `internal/routing/README.md`); `api/dns/handler.go` still snapshots DNS settings until certificate migration completes.
- **Certificates & ACME state** (`api/acme/storage.go`, `api/certificates/manager.go`) store certificate metadata, renewal status, and custom uploads.
- **Self-update coordination** (`api/selfupdate/executor.go`) locks rolling updates and records progress to unblock dashboards/CLI calls.
- **Mods knowledge base** (`internal/mods/kb_locks.go`, `internal/mods/kb_integration.go`) uses sessions for coarse locking and retry loops during KB writes.
- **Template + config fallbacks** (`internal/orchestration/kv.go`, legacy watchers) still default to Consul KV when overriding embedded templates.

## Why NATS JetStream
- **Key-Value Buckets** validated via [Key-Value Intro (Go)](https://natsbyexample.com/examples/kv/intro/go): revision metadata, optimistic concurrency (`Update` with expected sequence), and long-lived watchers that emit an initial catch-up sentinel before live updates.
- **Object Store** confirmed in [Object-Store Intro (Python)](https://natsbyexample.com/examples/os/intro/python): large payload chunking, metadata watch hooks, and streaming reads suitable for certificate bundles or build artifacts.
- **Streams & Durable Consumers** shown in [Pull Consumers (Go)](https://natsbyexample.com/examples/jetstream/pull-consumer/go): demand-driven fetch, durable offsets, and multi-subscriber fan-out without custom HTTP pollers.
- **Work-Queue Retention** illustrated by [Work-Queue Stream (Go)](https://natsbyexample.com/examples/jetstream/workqueue-stream/go): single-delivery semantics with enforcement against overlapping subjects—ideal for self-update executors or Mods KB maintenance tasks.
- **Operational Fit**: One NATS cluster services KV, object, and stream workloads; leaf nodes/superclusters can extend lane D without re-architecting discovery.

## Patterns from NATS by Example
- **Revision-aware CAS**: The KV example highlights treating `Revision()` as the compare-and-set token. Mirror this when porting `api/consul_envstore` so concurrent edits fail fast with `nats: wrong last sequence` instead of clobbering values.
- **Watcher Lifecycle**: `Watch()` delivers a nil entry signalling the backlog is drained before emitting live updates. Builder pods should wait for this sentinel to avoid racing old env snapshots.
- **Streamed Payloads**: Object Store readers iterate 128 KiB chunks; use identical buffering when migrating certificate uploads/downloads to prevent memory spikes.
- **Single-Delivery Queues**: Work-queue streams reject overlapping consumer filters (`err_code=10099`). Encode that rule in Mods/self-update consumer provisioning to avoid accidental duplicate processing.
- **Demand Fetchers**: Pull consumers fetch batches, process, ack, and can be durable or ephemeral. The CLI/event subsystem can borrow this to implement pause/resume telemetry viewers without losing offset.

## Migration Plan
### Phase 0 — Foundations
- ✅ **COMPLETED (2025-09-24)** `roadmap/nats/01-jetstream-cluster.md` – Deployed the JetStream cluster via `platform/nomad/jetstream.nomad.hcl`, published the operator runbook, and wired Traefik/CoreDNS so clients reach `nats.ploy.local:4222`.
- ✅ **COMPLETED** `roadmap/nats/02-kv-adapter.md` – JetStream-backed KV adapter documented with feature-flag wiring, adapter implementation steps, and test coverage expectations.
- Build integration tests that run against ephemeral NATS (docker-compose) mirroring existing Consul KV unit coverage.

### Phase 1 — Config & Environment State
- ✅ **COMPLETED** `roadmap/nats/03-envstore-dual-write.md` – Env store dual-write rollout guide detailing Consul + JetStream writes, metrics, and validation strategy.
- ✅ **COMPLETED** `roadmap/nats/04-envstore-watchers.md` – JetStream read cutover plan with watcher-driven cache invalidation, CAS handling, and documentation steps.
- Promote JetStream as primary after validation; retire Consul dependency for env store reads.

### Phase 2 — Routing & Certificate Metadata
- ✅ **COMPLETED (2025-11-04)** `roadmap/nats/05-routing-object-store.md` – Persist routing metadata in JetStream object storage and drive Traefik updates via events.
- ✅ **COMPLETED (2025-09-22)** `roadmap/nats/06-certificate-metadata.md` – Migrate certificate metadata and renewal flows to JetStream with broadcast notifications.

### Phase 3 — Controller Coordination & Locks
- ✅ **COMPLETED (2025-09-25)** `roadmap/nats/07-selfupdate-workqueue.md` – Move self-update coordination onto a JetStream work-queue stream with status events and full `ploy_updates_*` metrics.
- ✅ **COMPLETED (2025-09-25)** `roadmap/nats/08-mods-kb-locks.md` – Replace Mods KB locks with JetStream CAS, lifecycle events, and make JetStream the default locking backend.

### Phase 4 — Event Fabric & Cleanup
- `roadmap/nats/09-event-fabric.md` – Consolidate platform telemetry on JetStream streams with pull-consumer clients.
- `roadmap/nats/10-health-cleanup.md` – Extend health probes, retire Consul KV code, and update documentation to reflect the migration.

## Documentation Discipline
After closing any stage or task, refresh impacted READMEs and `docs/*` entries so the documentation set tracks the JetStream-backed behaviour.

## Event-Driven Simplifications
- **Environment Propagation**: KV watchers (Key-Value Intro) let lanes rebuild builder env caches instantly instead of time-based cache invalidation (`api/consul_envstore`).
- **Routing Updates**: Publishing `routing` stream events removes the need for Traefik to poll Consul; sidecars subscribe and apply updates atomically after watcher catch-up.
- **Certificate Rotation**: Renewal events on `certs.renewed` notify Traefik + apps to reload TLS without manual hooks (`api/certificates/manager.go`), streaming bundles through Object Store chunk readers.
- **Self-Update Monitoring**: CLI subscribes to `updates.control-plane.status` via durable pull consumer to render live progress instead of polling the REST endpoint backed by Consul KV.
- **Mods Telemetry**: Mods runner emits structured events to JetStream, enabling multiple observers (controller UI, analytics) without HTTP fan-out; pull consumers distribute load safely.
- **Knowledge Base Maintenance**: Maintenance jobs watch `mods.kb.lock.*` subjects to trigger compaction when writers finish, avoiding periodic Consul scans; single-delivery streams guarantee one worker per lock release.
- **Build Pipeline Signals**: Orchestration layer pushes `nomad.alloc.ready` events to JetStream; CLI uses demand fetchers to tail status without busy loops.

## Risks & Open Questions
- Migration sequencing must ensure no data loss; plan KV backfill scripts (Consul → JetStream) with replayable snapshots.
- JetStream storage sizing and retention policies need validation for certificate objects, Mods telemetry volume, and work-queue backlog length.
- Access control (NATS accounts/creds) must replicate current Consul ACL boundaries; audit integration still TBD.
- Service discovery remains on Consul; evaluate long-term plan (Nomad service discovery, Traefik native providers) after KV migration stabilises.

## Related Documentation
- `docs/REPO.md` – Repository map and subsystem pointers.
- `docs/FEATURES.md` – Platform capability matrix (update once JetStream GA).
- `internal/orchestration/README.md` – Details on current Consul coordination utilities.
- `internal/mods/README.md` – Mods subsystem architecture and coordination patterns.
- `roadmap/README.md` – Broader roadmap context and prior Consul externalisation milestones.
