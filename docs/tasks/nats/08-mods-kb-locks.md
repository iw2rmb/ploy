# Mods Knowledge Base Locking Migration

> **Status (2025-09-25):** Completed — JetStream locking is mandatory and the
> legacy `PLOY_USE_JETSTREAM_KV=false` override has been removed.

## What to Achieve

- Eliminate Consul sessions/locks for Mods knowledge base (KB) writers and rely
  on JetStream Key-Value buckets for optimistic locking and state handoff.
- Emit lock lifecycle events on JetStream subjects so maintenance workers react
  immediately to lock release/expiry.
- Remove Consul-specific feature flags and metrics in favour of JetStream-native
  observability.

## Why It Matters

- Consul sessions require heartbeats and are sensitive to transient network
  jitter, which stalls KB writes and creates orphaned sessions.
- JetStream KV provides revision-aware CAS semantics so writers know exactly
  which revision they own; retries become deterministic.
- Subject-based events unlock reactive maintenance (compaction, index refresh)
  and operator visibility without polling Consul.
- Cutting the Consul dependency keeps Mods aligned with the broader JetStream
  migration and simplifies rollout to additional deployment lanes.

## Current State & Pain Points

- JetStream KV owns KB locking across all lanes; Consul-based paths have been
  removed from `internal/mods` packages.
- Maintenance routines (`internal/mods/kb_maintenance.go`,
  `internal/mods/kb_integration.go`) subscribe to JetStream events instead of
  polling Consul.
- Operator guidance is aligned on JetStream tooling
  (`docs/runbooks/mods-kb-locks.md`).

## Prerequisites

- JetStream cluster/workload credentials provisioned via roadmap stages 01-06
  and distributed to Mods API workers (`NATS_CREDS_MODS` secret).
- The shared JetStream KV adapter (`internal/orchestration/kv.go`) exposes the
  `mods` bucket with CAS helpers delivered in `docs/tasks/nats/02-kv-adapter.md`.
- Mods services already use the global JetStream connection pool (see
  `internal/mods/init.go`) and metrics exporter for connection state.

## Target Architecture

- **KV Bucket**: `mods_kb_locks`
  - Keys: `writers/<kb-id>` (kb-id = tenant/app/mod identifier).
  - Metadata: `Revision()` acts as lease token; `Value` encodes lock owner
    (`alloc-id`, host, worker ID) and expiry timestamp.
  - TTL enforcement via `MaxAge=10m` with proactive extension by the holder;
    stale entries expire without external cleanup.
- **Subjects**:
  - `mods.kb.lock.acquired.<kb-id>` and `mods.kb.lock.released.<kb-id>` events
    published alongside KV mutations.
  - Optional `mods.kb.lock.expired.<kb-id>` event when a worker notices an
    expired revision during CAS.
- **Consumers**:
  - Maintenance worker durable consumer `mods-kb-maintenance` watching
    `mods.kb.lock.*` to trigger compaction/index refresh.
  - Observability consumer `mods-kb-observer` (CLI/UI) for real-time monitoring.

## Implementation Steps

1. **Finalize JetStream Manager**
   - Complete `NewJetstreamKBLockManager()` to provision/access the
     `mods_kb_locks` bucket, capture structured logging, and expose clear error
     handling.
   - Remove Consul client dependency from `Lock` struct; replace `SessionID`
     with `Revision` and `LeaseExpiresAt` fields.
2. **CAS-based Acquire/Release**
   - Implement `AcquireLock` using `kv.Create()` for first acquisition and
     `kv.Update()` when caller extends an existing revision.
   - Treat `nats.ErrKeyExists` / `nats.ErrWrongLastSequence` as contention;
     include jittered backoff, capped at `LockConfig.MaxWait`.
   - On release, call `kv.Delete(key, expectedRevision)`; fall back to
     `kv.Purge()` if revision mismatch indicates prior expiry.
3. **Lifecycle Events**
   - Introduce `publishLockEvent(ctx, subject, payload)` helper in
     `internal/mods/kb_locks.go` using the shared JetStream connection.
   - Publish `acquired` after successful CAS; publish `released` before delete
     ack; emit `expired` when detecting stale revisions.
   - Payload fields: `kb_id`, `revision` (uint64), `owner` (allocator ID),
     `ts` (RFC3339 timestamp).
4. **Retry & Backoff Strategy**
   - Replace Consul polling loops in `TryWithLockRetry` with CAS-aware retries:
     jitter between 200–500ms, respect `LockConfig.RetryBudget`, surface
     `context.DeadlineExceeded` clearly.
   - Export histogram metric for wait duration and count lock contention events.
5. **Maintenance Hooks**
   - Update `internal/mods/kb_maintenance.go` to subscribe to
     `mods.kb.lock.released.*` and trigger compaction immediately instead of
     periodic scans.
   - Ensure maintenance jobs acknowledge events and de-duplicate using message
     IDs derived from revision numbers.
6. **Configuration Cleanup**
   - Remove `PLOY_USE_JETSTREAM_KV` toggle once JetStream path is default; keep
     temporary override flag for rollback documented in runbook. _(Completed Sep
     25 2025 — toggle removed, Consul fallback disabled.)_
   - Delete Consul ACL policies and Terraform entries referencing `kb/locks/*`.
7. **Documentation & Runbooks**
   - Update `internal/mods/README.md` with JetStream locking diagrams, CLI
     snippets for inspecting bucket state (`nats kv info mods_kb_locks`).
   - Add new runbook (`docs/runbooks/mods-kb-locks.md`) covering lock
     inspection, manual release via KV delete, and interpreting metrics.

## Rollout Strategy

- **Stage 1 – Shadow Writes**: Enable dual write (Consul + JetStream) behind
  feature flag for non-critical KBs; compare acquisition latency and lock
  durations.
- **Stage 2 – Writer Cutover**: Switch Mods writers to JetStream-only, keep
  Consul read-only for fallback during 1 sprint.
- **Stage 3 – Maintenance/Event Adoption**: Deploy maintenance consumers and
  retire Consul cron; monitor event delivery lag and compaction latency.
- **Stage 4 – Cleanup**: Purge stale Consul keys, remove feature flag, promote
  new runbooks.

## Operations

- Logs: Structured entries on acquisition attempts, revision numbers, release
  paths, and CAS failures (include masked connection IDs).
- Alerts: Fire on contention > 5 consecutive failures, lock held > 5m without
  release event, or event consumer lag > 60s.
- Tooling: Provide `cmd/ploy mods locks tail --kb <id>` to stream event subjects
  and surface lock health alongside Mods telemetry.

## Risks & Mitigations

- **JetStream Outage**: Writers now fail-fast; incident response requires
  restoring JetStream availability rather than toggling to Consul. Document
  manual drain procedures and circuit breakers in runbooks.
- **Stale Locks**: TTL-based expiry plus release events guard against stuck
  locks; schedule watchdog to scan for buckets lacking release events beyond
  SLA.
- **Credential Scope**: Limit Mods creds to `mods.kb.*` subjects; rotate via
  existing secret pipeline and track with `NATS_CREDS_MODS` expiry monitors.
- **High Churn**: Size bucket MaxValues appropriately; consider sharding
  (`mods_kb_locks.<region>`) if contention increases.

## Expected Outcome

- Mods KB locking uses JetStream CAS semantics exclusively, unlocking
  deterministic retries and leaner code.
- Maintenance jobs react to release events within seconds, reducing KB follow-up
  latency.
- Consul KV usage for Mods is removed, aligning with the roadmap's Phase 3
  objective and lowering operational overhead.

## Tests

- **Unit**: Cover CAS success/failure paths with JetStream fakes, verify event
  publication, and ensure retry/backoff respects deadlines
  (`internal/mods/kb_locks_test.go`).
- **Integration**: Spin up ephemeral JetStream via test harness; run concurrent
  writers to validate contention behaviour and event sequencing.
- **E2E**: Execute Mods KB update scenario (e.g.
  `tests/e2e/mods/TestModsKnowledgeBaseUpdate`) targeting lane D; confirm
  exclusive writes, timely maintenance triggers, and absence of Consul keys.
