# Control Plane & Job Scheduling Design Spec

- **Identifier**: `roadmap-control-plane`
- **Status**: [ ] Draft · [ ] In progress · [x] Completed — last updated 2025-10-21
- **Linked Tasks**:
  - [x] `roadmap-control-plane-01` – `docs/tasks/roadmap/01-control-plane-and-job-scheduling.md`
- **Blocked by**:
  - `docs/v2/etcd.md`
  - `docs/v2/queue.md`
  - `docs/v2/job.md`
- **Unblocks**:
  - `roadmap-mod-step-runtime` / `docs/design/mod-step-runtime/README.md`
  - `roadmap-control-plane-02` / `docs/tasks/roadmap/02-mod-step-runtime.md`
  - `roadmap-control-plane-07` / `docs/tasks/roadmap/07-job-observability.md`
- **Last Verification**: 2025-10-21 —
  `go test ./internal/controlplane/...`, `go test -tags integration ./tests/integration/controlplane`,
  `staticcheck ./internal/controlplane/...`, `make lint-md` (fails on pre-existing roadmap task lint debt)
- **Upstream Dependencies**:
  - `../../v2/etcd.md`
  - `../../v2/queue.md`
  - `../../v2/job.md`
- Etcd v3.6 documentation on optimistic concurrency & leases
  (<https://etcd.io/docs/v3.6/dev-guide/api_grpc_gateway/#service-lease>,
  <https://etcd.io/docs/v3.6/dev-guide/interacting_v3/>)

## Intent

Deliver a workstation-first control plane that schedules Mods steps without Grid. The service must
expose submission, status, and worker-claim APIs backed entirely by etcd. Jobs live once in the
queue, multiple workers may compete safely, and state survives node failure.

## Context

Today the CLI depends on Grid Workflow RPC to orchestrate Mods. That forces beacon/Grid availability
and complicates workstation execution. Ploy v2 already documents an etcd-centric queue, but no code
exists. We need a control plane aligned with those docs: etcd as source of truth, IPFS for payload
storage, and workstation nodes executing containers.

## Goals

- Implement an etcd-backed job scheduler with optimistic transactions and lease-based job claims.
- Persist full job lifecycle (queued → running → succeeded/failed/inspection-ready) with retention controls.
- Remove Grid leader-election scaffolding from scheduler pathways and expose native APIs for CLI + nodes.

## Non-Goals

- Replacing Mods runtime container execution (covered by roadmap-control-plane-02).
- Implementing observability streams beyond job status metadata (handled by roadmap-control-plane-07).
- Shipping beacon bootstrapping or multi-cluster federation; scope is single cluster.

## Current State

- No scheduler service exists. Workflow runner talks to Grid only.
- `docs/v2/etcd.md` describes desired key prefixes without implementation.
- No mechanism to schedule jobs locally; no runbooks for job recovery.

## Proposed Architecture

### Overview

1. `ploy ctl` (future) or CLI submits a Mod step to the control plane HTTP API (`POST /v2/jobs`).
2. Control plane writes the job record (`mods/<ticket>/jobs/<job-id>`) and enqueues to
   `queue/mods/<priority>/<job-id>`.
3. Worker nodes POST to `/v2/jobs/claim` with their node ID; the server executes an etcd transaction
   to delete the queue key, mark the job `running`, and persist a lease-bound claim record.
4. Worker periodically renews the lease with `POST /v2/jobs/{job-id}/heartbeat`. Lease TTL ensures
   stuck jobs revert to `queued` automatically.
5. Worker finishes via `POST /v2/jobs/{job-id}/complete`. Control plane updates status, timestamps,
   artifacts, and writes GC markers. Inspection flows can mark `inspection_ready` instead of
   deleting state.
6. Garbage collector service scans jobs past retention windows and prunes them.

### Interfaces & Contracts

- HTTP JSON APIs (versioned under `/v2`):
  - `POST /v2/jobs`: submits work (ticket, step metadata, retry budget, priority).
  - `GET /v2/jobs/{job-id}`: returns full job state (requires `ticket` query param for lookup).
  - `GET /v2/jobs?ticket=<ticket>`: lists job summaries for CLI status queries.
  - `POST /v2/jobs/claim`: workers provide `node_id`; returns claim result or `"empty"` status.
  - `POST /v2/jobs/{job-id}/heartbeat`: extends the claim lease (`ticket` + `node_id`).
  - `POST /v2/jobs/{job-id}/complete`: records terminal status (`succeeded`, `failed`,
    `inspection_ready`).
- gRPC may be introduced later but initial implementation keeps net/http.
- Authentication is out of scope for this task; rely on network isolation.

### Data Model & Persistence

- **Key prefixes** (JSON payloads):
  - `queue/mods/<priority>/<job-id>` — queued entries.
  - `mods/<ticket>/jobs/<job-id>` — canonical job state; fields:
    - `state`: `queued|running|succeeded|failed|inspection_ready`
    - `lease_id`: etcd lease ID currently holding the job (0 if none).
    - `claimed_by`: node ID
    - timestamps: `enqueued_at`, `claimed_at`, `completed_at`, `expires_at`
    - `retry_attempt`, `max_attempts`
    - `artifacts`, `logs`, `error`
  - `nodes/<node-id>/status` — heartbeat data (`last_seen`, version, capacities).
  - `leases/jobs/<job-id>` — ephemeral keys bound to leases (value: summary for debugging).
  - `gc/jobs/<job-id>` — markers for retention scanning.
- **Optimistic concurrency**: every mutation uses `clientv3.Txn` with
  `Compare(ModRevision(jobKey) == expected)`.
- **Lease TTLs**: claims create a lease with configurable TTL (default 120s). TTL expiry triggers a
  watch; the control plane transitions the job to `queued` and increments the retry counter.
- **Retention**: succeeded or failed jobs move to the `gc` prefix with an expiry timestamp. The GC
  worker removes queue entries and the job record after that window.

### Failure Modes & Recovery

- **Worker crash**: lease expires, watcher re-queues the job by clearing `lease_id`, setting state
  `queued`, incrementing `retry_attempt`, and re-creating the queue key.
- **Control plane crash**: on restart, watchers resume; outstanding leases continue because etcd keeps
  them. Jobs with expired TTLs are detected on the next watch sync.
- **Duplicate claims**: prevented by txn comparing queue key and job revision.
- **Queue backlog**: documented metrics and runbook instruct operators to inspect `queue/mods/*`.
- **Stuck `inspection_ready`**: GC retains jobs for the configured window; runbook describes manual
  deletion.

## Dependencies & Interactions

- Introduces new Go packages:
  - `internal/controlplane/etcd` — client helpers, key builders, codecs.
  - `internal/controlplane/scheduler` — job service, claim logic.
  - `internal/controlplane/httpapi` — HTTP handlers.
- `internal/workflow/runtime` updated to select the new adapter; Grid adapter remains but scheduler
  flow no longer references Grid leader election.
- CLI wiring (follow-up task) will call new APIs.
- Requires etcd client v3.6.

## Risks & Mitigations

- **Lease misconfiguration leads to premature requeue**
  - Impact: Double execution
  - Mitigation: Expose config knobs, default to 2× expected step heartbeat, add integration tests.
- **etcd contention under high concurrency**
  - Impact: Increased latency
  - Mitigation: Batch queue scans (limit) and rely on etcd linearizable reads only for claims.
- **GC removing active jobs**
  - Impact: Lost diagnostics
  - Mitigation: GC filters states succeeded / failed / inspection_ready older than retention window.

## Observability & Telemetry

- Metrics: queue depth, claim latency, lease expirations, retries.
- Logs: structured events on submission, claim, completion, failure, requeue.
- Traces: optional but include job ID tags.
- Health endpoint: `/v2/health` returns etcd connectivity status and backlog size.

## Test Strategy

- **Unit**: scheduler txn builder, lease management, retry state machine, GC filters.
- **Integration**: embedded etcd cluster with >1 worker goroutine racing to claim the same job;
  verify single claim, lease expiry requeue, heartbeat renewal.
- **E2E**: defer to roadmap-control-plane-02 once CLI integration lands.

## Rollout Plan

1. Land scheduler service + HTTP API with feature flag `PLOY_SCHEDULER_MODE`.
2. Enable integration tests in CI using embedded etcd.
3. Update CLI to target new control plane.
4. Decommission Grid adapter after CLI flips default.

## Open Questions

- Do we require per-tenant isolation in keyspace? Pending product decision.
- Should inspection state requeue automatically after expiry? TBD.
- AuthN/AuthZ strategy for control plane APIs (deferred to security review).

## Follow-Up Work (2025-10-21)

- [ ] Planned – [roadmap-control-plane-02 Mod Step Runtime](../../tasks/roadmap/02-mod-step-runtime.md)
- [ ] Planned – [roadmap-control-plane-07 Job Observability](../../tasks/roadmap/07-job-observability.md)

Status verification: task entries reviewed on 2025-10-21.

## References

- `docs/v2/etcd.md`
- `docs/v2/queue.md`
- `docs/v2/job.md`
- Etcd lease & concurrency docs
  (<https://etcd.io/docs/v3.6/dev-guide/api_grpc_gateway/#service-lease>,
  <https://etcd.io/docs/v3.6/dev-guide/interacting_v3/>)
