# Self-Update Work-Queue Migration

> **Status (2025-09-25):** Completed — JetStream queue and metrics
> instrumentation (`ploy_updates_*`) are in place.

## What to Achieve

- Replace Consul sessions + KV status records with a JetStream work-queue stream
  (`updates.control-plane`) governing controller self-update tasks.
- Deliver real-time progress via JetStream status events so the CLI/UI renders
  updates without polling REST endpoints backed by Consul.
- Encode retry, rollback, and concurrency rules directly in JetStream consumer
  configuration to keep a single authoritative executor per deployment lane.

## Why It Matters

- Consul sessions are brittle during network partitions and require periodic
  renewals; JetStream work-queue retention guarantees exactly-one delivery with
  explicit acknowledgement windows.
- Streaming progress removes the polling pressure on the API and exposes richer
  telemetry (phase, percent, executor identity) to operators.
- Consolidating coordination on JetStream aligns with the broader KV/object
  store migration and simplifies the controller binary distribution story.

## Where Changes Will Affect

- `api/selfupdate/handler.go`, `api/selfupdate/executor.go`,
  `api/selfupdate/utils.go` – task submission, status publication, retry logic,
  and executor lifecycle management.
- `api/server/initializers_network.go`, `api/server/routes.go` – dependency
  wiring for the JetStream connection, stream bootstrap, and HTTP handler
  registration.
- CLI + dashboard consumers (e.g. `cmd/ploy`, `internal/ui/updates`) that
  currently poll `/v1/api/update/status` – replace polling with JetStream pull
  consumers.
- Documentation touchpoints: `docs/deployments.md`, `docs/FEATURES.md`,
  `roadmap/deploy.md`, any operator runbooks describing self-update workflows.

## Prerequisites

- JetStream control plane cluster, account configuration, and credentials
  completed in roadmap stages 01–06 (cluster bootstrap, KV adapter, env store,
  routing, and certificate migrations).
- Controller process already linked with the JetStream connection pool and
  credential rotation helpers delivered by `roadmap/nats/02-kv-adapter.md`.
- Distribution pipeline deposits controller binaries + metadata in the artefact
  store (SeaweedFS → JetStream object store) so the executor retrieves versioned
  payloads deterministically.
- Observability foundation: metrics sink (`ploy_metrics_exporter`) and log
  shipping for controller pods already reading JetStream connection stats.

## Stream & Status Model

- **Work-Queue Stream**: `updates.control-plane`
  - Subjects: `updates.control-plane.tasks.<lane>` (lane = `d`, `dev`, etc.).
    Allows per-lane isolation; add `updates.control-plane.tasks.default` for
    single-lane deployments.
  - Retention: `WorkQueue`, replicas: 3, `MaxAckPending=1` per lane consumer,
    `MaxDeliver=5`, `AckWait=2m` (tunable per strategy).
  - Payload fields:
    - `deployment_id` (UUID), `target_version` (e.g. `2025.11.0`).
    - `strategy` (`rolling`, `blue_green`, or `emergency`).
    - `lane`, `window` (maintenance window metadata), `submitted_by`
      (`user`/`system`), `metadata` (strategy extras).
  - Headers include `Nats-Msg-Id=<deployment_id>` to dedupe submissions.
    `X-Ploy-Trigger` captures API origin (`api`, `cli`, `scheduler`).
- **Executor Durable Consumer**: `updates-control-plane-<lane>` with filter
  `updates.control-plane.tasks.<lane>` and `MaxAckPending=1` to enforce
  single-runner semantics. Configure `DeliverPolicy=All` so replays pick up
  stalled tasks after restart.
- **Status Stream**: `updates.control-plane.status`
  - Subjects: `updates.control-plane.status.<deployment_id>` carrying state
    transitions.
  - Retention: `Limits`, `MaxMsgsPerSubject=128`, `MaxAge=72h` (enough for
    post-mortems).
  - Payload fields: `deployment_id`, `phase` (`preparing`, `downloading`,
    `deploying`, `validating`, `completed`, `failed`), `progress` (0–100),
    `message`, `executor`. Optional `checkpoint` fields reference backup
    artefacts or rollback coordinates.
  - Durable consumers: `updates-status-cli`, `updates-status-ui`. Use pull
    consumers so clients request on-demand batches and resume with stored
    sequence offsets.
- **Advisory Events**: Publish `updates.control-plane.audit` events on
  submission/ack/failure for audit trails and alerting.

## How to Implement

1. **Bootstrap JetStream Resources**
   - Extend the controller startup (`api/server/initializers_network.go`) to
     idempotently provision the `updates.control-plane` work-queue stream and
     the `updates.control-plane.status` stream when absent.
   - Surface metrics (`ploy_updates_js_bootstrap_total`, labelled by stream) and
     fail fast if provisioning fails; tie bootstrap to the existing JetStream
     connection lifecycle.
2. **Define Submission API Contract**
   - Update `api/selfupdate/handler.go` to validate incoming requests, translate
     them into work-queue payloads, and publish to
     `updates.control-plane.tasks.<lane>` with the correct headers.
   - Reject duplicate submissions by checking for existing JetStream message IDs
     (use `MsgId` header and handle `nats: duplicate message` gracefully).
   - Persist submission metadata (who/why) via status stream seed message
     (`phase=queued`).
3. **Replace Consul Session Acquisition**
   - Remove `createUpdateSession` and `kv.Acquire` usage; instead, the executor
     obtains work by fetching from its durable consumer.
   - Enforce single-runner by using `Fetch(1)` loops and acknowledging only
     after completion. On fatal errors, send `Nak` with delay to trigger
     redelivery after `AckWait`.
4. **Status Publication Pipeline**
   - Build a helper (`internal/selfupdate/status.go`) that wraps JetStream
     PublishAsync to `updates.control-plane.status.<deployment_id>` with
     standard headers (`X-Ploy-Phase`, `X-Ploy-Progress`).
   - Replace `updateStatus` to publish to JetStream while keeping the HTTP
     handler’s in-memory cache for backward compatibility during rollout.
   - Seed a terminal `failed` or `completed` event before acking the task so
     observers see final state even if the executor crashes mid-ack.
5. **Executor Lifecycle & Idempotency**
   - Track executor identity (Nomad allocation ID or hostname) and include it in
     status events.
   - Record rollback checkpoints in the status payload; ensure
     emergency/rollback flows produce matching events.
   - On startup, drain outstanding tasks (fetch without ack) to detect stuck
     deployments and emit `phase=aborted` before requesting manual intervention.
6. **Client Consumption Changes**
   - Update CLI to open a pull consumer on `updates.control-plane.status`
     filtered by `deployment_id`; map phases to progress bars/log lines.
     _(Implemented via `internal/cli/updates` and `ploy updates tail`, Sep 25
     2025.)_
   - For dashboards, embed the same consumer via WebSocket or SSE bridge;
     include exponential backoff when the JetStream connection drops.
   - Maintain REST `/update/status` endpoint temporarily by proxying JetStream
     history (fetch latest message for each deployment) to support older
     clients.
7. **Configuration & Secrets**
   - Inject JetStream credentials into the self-update handler via existing
     secret distribution (`NATS_UPDATES_CREDS`), separate from other consumers
     so permissions are scoped to `updates.control-plane.*` subjects.
   - Document required environment variables (`PLOY_CONTROLLER`,
     `NATS_CREDS_UPDATES`) and update Ansible/ployman templates to mount them in
     the API allocation.
8. **Consul Cleanup**
   - Drop Consul session creation, KV locks, and related ACL policies
     (`ploy/selfupdate/*`).
   - Remove feature flags or fallback config referencing Consul-based locking.
   - Update runbooks to direct operators to JetStream for queue inspection
     (`nats stream info updates.control-plane`).

## Observability & Operations

- Metrics: `ploy_updates_tasks_submitted_total`,
  `ploy_updates_executor_duration_seconds`,
  `ploy_updates_status_published_total`, `ploy_updates_redeliveries_total`,
  `ploy_updates_status_consumer_lag_seconds`.
- Logs: Structured events on submission, executor start/finish, redelivery, and
  status publication (mask sensitive metadata).
- Alerts: page on high redelivery counts, tasks older than SLA (no status
  updates for >5m), or consumer lag exceeding threshold.
- Runbooks:
  - Add `docs/runbooks/selfupdate-jetstream.md` detailing stream inspection,
    forced nack, and manual rollback. _(Added Sep 25 2025.)_
  - Update `docs/deployments.md` with instructions on tailing status events and
    requeueing tasks.
- Tooling: Provide `cmd/ploy updates tail` CLI command that binds to status
  consumer for ad-hoc monitoring.

## Deliverables

- JetStream-backed self-update handler and executor with status streaming and
  removal of Consul dependencies.
- Updated CLI/UI clients consuming JetStream status feeds and optional
  compatibility layer for legacy REST polling.
- Documentation updates across `docs/FEATURES.md`, `docs/deployments.md`,
  `roadmap/deploy.md`, and new runbook content.
- Telemetry (metrics/logs) and alert rules capturing queue health and executor
  behaviour.
- Migration checklist for disabling Consul ACL entries and validating JetStream
  stream provisioning in staging/production.

## Tests

- **Unit**: Executor fetch/ack logic (including NAK delay), status publisher
  formatting, submission validation (duplicate detection, payload schema) using
  JetStream mocks.
- **Integration**: In-memory JetStream instance verifying work-queue semantics,
  redelivery after crash, and status stream catch-up for new consumers.
- **E2E**: Trigger controller self-update in staging via CLI, confirm status
  events stream to UI, ensure rollback path publishes terminal events, and
  confirm no Consul keys remain.
