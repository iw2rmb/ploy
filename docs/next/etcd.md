# etcd Data Model

Ploy Next uses etcd as the authoritative control-plane store for queueing, job state, node capacity,
configuration, and IPFS metadata. This document provides an overview of the key prefixes and the
contracts expected under each.

## Overview of Prefixes

- **`config/`** — Cluster-scoped configuration (GitLab API keys, feature flags). Managed via
  `ploy config set/show`.
- **`queue/<kind>/...`** — Waiting jobs (see [docs/next/queue.md](queue.md)). `<kind>` is `mods` or
  `buildgate`; entries store resource requirements, priority, and retry counters.
- **`mods/<ticket>/jobs/<job-id>`** — Durable job records for Mod tickets. Includes lifecycle
  state, timestamps, lease metadata, artifacts, and retry counters (see
  [docs/next/job.md](job.md)).
- **`buildgate/<ticket>/jobs/<job-id>`** — Optional if build gate jobs are stored outside the Mod
  tree. Same schema as Mod jobs but flagged `type: buildgate`.
- **`leases/jobs/<job-id>`** — Ephemeral keys bound to etcd leases tracking active job claims.
- **`gc/jobs/<job-id>`** — Garbage-collection markers stamped when jobs enter a terminal state.
- **`nodes/<node-id>/capacity`** — Node capacity snapshots. See [docs/next/queue.md](queue.md) for
  polling/updates.
- **`nodes/<node-id>/status`** — Health info (heartbeat, current version tag). Populated by `ployd`
  heartbeats; used by the control plane.
- **`ipfs/peers/<node-id>`** — IPFS Cluster peer metadata (peer ID, multiaddr, last seen; see
  [docs/next/ipfs.md](ipfs.md)).
- **`artifacts/<cid>`** — Optional metadata for artifacts published outside job context. Tracks
  orphaned CIDs or global references.
- **`gc/pending/<job-id>`** — Jobs pending garbage collection. Set by the GC controller prior to
  deletion (see [docs/next/gc.md](gc.md)).

## Detailed Contracts

### Configuration (`config/`)

- Keys are simple strings (e.g., `config/gitlab.api_key`).
- Values are JSON blobs or plain strings depending on the setting. The CLI merges beacon values with
  local overrides as described in [docs/next/cli.md](cli.md).
- Changes take effect immediately; nodes may watch the prefix to refresh local configuration.

### Queue (`queue/<kind>/<priority>/<job-id>`)

- Values include resource requirements and metadata:

  ```json
  {
    "ticket": "mod-123",
    "step_id": "apply-1",
    "cpu": 1000,
    "mem": 512,
    "retry": 0,
    "enqueued_at": "2025-10-08T12:34:56Z"
  }
  ```

- Job claimers must perform transactional delete + job record mutation (see
  [queue.md](queue.md) for details). Claims attach a lease so the key automatically expires if a
  worker disappears.
- No extra data should be stored under the prefix to keep range scans efficient.

### Job Records (`mods/<ticket>/jobs/<job-id>`)

- Schema includes:

  ```json
  {
    "id": "job-8a9f",
    "ticket": "mod-123",
    "step_id": "apply-1",
    "priority": "default",
    "state": "queued" | "running" | "succeeded" | "failed" | "inspection_ready",
    "created_at": "2025-10-08T12:34:56Z",
    "enqueued_at": "2025-10-08T12:34:56Z",
    "claimed_at": "2025-10-08T12:35:00Z",
    "completed_at": "2025-10-08T12:45:00Z",
    "expires_at": "2025-10-11T12:45:00Z",
    "claimed_by": "node-7",
    "lease_id": 1234567,
    "lease_expires_at": "2025-10-08T12:37:00Z",
    "retry_attempt": 0,
    "max_attempts": 2,
    "artifacts": {
      "diff_cid": "bafy..."
    },
    "bundles": {
      "logs": {
        "cid": "bafy-log",
        "digest": "sha256:bundle",
        "ttl": "72h",
        "expires_at": "2025-10-11T12:45:00Z",
        "retained": true
      }
    },
    "retention": {
      "retained": true,
      "ttl": "72h",
      "expires_at": "2025-10-11T12:45:00Z",
      "bundle": "logs",
      "bundle_cid": "bafy-log"
    },
    "node_snapshot": {
      "node_id": "node-7",
      "capacity": {
        "cpu_free": 6000,
        "mem_free": 8192,
        "heartbeat": "2025-10-08T12:35:00Z",
        "revision": 42
      },
      "capacity_at": "2025-10-08T12:35:00Z",
      "status": {
        "phase": "ready",
        "heartbeat": "2025-10-08T12:35:05Z"
      },
      "status_at": "2025-10-08T12:35:05Z"
    },
    "error": {
      "reason": "lease_expired",
      "message": "worker lost heartbeat"
    }
  }
  ```

- See [docs/next/job.md](job.md) for the lifecycle and updates.
- The scheduler persists `expires_at`, bundle retention metadata, and the latest node snapshot on
  every job mutation; `gc/jobs` updates will fan out through a watcher to keep the record's
  retention window in sync.

### Job Leases (`leases/jobs/<job-id>`)

- Keys are bound to etcd leases. Values capture `job_id`, `ticket`, and `priority` so the control
  plane can requeue jobs when the lease expires.
- When a worker disappears, the lease expires, the key is removed automatically, and the scheduler
  re-enqueues the job or marks it failed if `retry_attempt >= max_attempts`.

### Garbage Collection (`gc/jobs/<job-id>`)

- Written when jobs transition to `succeeded`, `failed`, or `inspection_ready`.
- Values include the final state and `expires_at` timestamp for retention controllers.
- The GC controller deletes the marker after removing the job record and associated artifacts.

### Node Capacity (`nodes/<node-id>/capacity`)

- Updated every 15 seconds by `ployd`.
- Transactional updates are required when claiming jobs to avoid race conditions (see
  [queue.md](queue.md)).
- Example entry:

  ```json
  {
    "cpu_free": 6000,
    "mem_free": 8192,
    "heartbeat": "2025-10-08T12:35:00Z",
    "revision": 42
  }
  ```

### Node Status (`nodes/<node-id>/status`)

- Contains heartbeat timestamp, runtime version tag, optional health flags.
- The control plane uses this to detect node outages and trigger failover.

### IPFS Metadata (`ipfs/peers/<node-id>`, `artifacts/<cid>`)

- `ipfs/peers` entries store peer ID, multiaddrs, last seen timestamp.
- `artifacts/<cid>` (if used) tracks global references to CIDs so the GC controller knows when a CID
  remains pinned for other jobs/tickets.

## Watchers & Notifications

- Control plane services may watch specific prefixes:
  - `queue/` for new work (scheduler).
  - `leases/jobs/` for lease expiry requeues.
  - `gc/jobs/` so job records inherit retention updates.
  - `nodes/<node-id>/status` for heartbeat monitoring and node health snapshots.
  - `config/` for configuration changes.
- Ensure watchers are scoped to specific prefixes to avoid excessive load on etcd.

## Summary

- etcd keys are well-scoped by function, allowing efficient scans/watchers.
- Jobs, queue entries, and node capacity updates must use transactions to maintain consistency.
- Lease expirations automatically surface through the `leases/jobs/` watcher so stuck jobs can be
  re-queued without manual intervention.
- IPFS metadata and log references stay lightweight (CIDs/digests) while payloads live in IPFS.
