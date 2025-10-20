# etcd Data Model

Ploy v2 uses etcd as the authoritative control-plane store for queueing, job state, node capacity,
configuration, and IPFS metadata. This document provides an overview of the key prefixes and the
contracts expected under each.

## Overview of Prefixes

- **`config/`** — Cluster-scoped configuration (GitLab API keys, feature flags). Managed via
  `ploy config set/show`.
- **`queue/<kind>/...`** — Waiting jobs (see [docs/v2/queue.md](queue.md)). `<kind>` is `mods` or
  `buildgate`; entries store resource requirements and metadata.
- **`mods/<ticket>/jobs/<job-id>`** — Job records for Mod tickets. Includes state, timestamps,
  artifact CIDs, log references, retry metadata (see [docs/v2/job.md](job.md)).
- **`buildgate/<ticket>/jobs/<job-id>`** — Optional if build gate jobs are stored outside the Mod
  tree. Same schema as Mod jobs but flagged `type: buildgate`.
- **`nodes/<node-id>/capacity`** — Node capacity snapshots. See [docs/v2/queue.md](queue.md) for
  polling/updates.
- **`nodes/<node-id>/status`** — Health info (heartbeat, current version tag). Populated by
  ploynode heartbeats; used by the control plane.
- **`ipfs/peers/<node-id>`** — IPFS Cluster peer metadata (peer ID, multiaddr, last seen; see
  [docs/v2/ipfs.md](ipfs.md)).
- **`artifacts/<cid>`** — Optional metadata for artifacts published outside job context. Tracks
  orphaned CIDs or global references.
- **`gc/pending/<job-id>`** — Jobs pending garbage collection. Set by the GC controller prior to
  deletion (see [docs/v2/gc.md](gc.md)).

## Detailed Contracts

### Configuration (`config/`)

- Keys are simple strings (e.g., `config/gitlab.api_key`).
- Values are JSON blobs or plain strings depending on the setting. The CLI merges beacon values with
  local overrides as described in [docs/v2/cli.md](cli.md).
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

- Job claimers must perform transactional delete + job creation (see [queue.md](queue.md) for
  details).
- No extra data should be stored under the prefix to keep range scans efficient.

### Job Records (`mods/<ticket>/jobs/<job-id>`)

- Schema includes:

  ```json
  {
    "job_id": "job-8a9f",
    "type": "mod_step" | "buildgate",
    "state": "queued" | "claimed" | "running" | "succeeded" | "failed" | "cancelled",
    "node": {
      "claimed_by": "node-7",
      "claimed_at": "2025-10-08T12:35:00Z"
    },
    "artifacts": {
      "diff_cid": "bafy...",
      "build_gate_cid": "bafy..."
    },
    "logs": {
      "cid": "bafy...",
      "digest": "sha256:..."
    },
    "retry": {
      "attempt": 0,
      "max": 2
    },
    "expires_at": "2025-10-15T12:35:00Z"
  }
  ```

- See [docs/v2/job.md](job.md) for the lifecycle and updates.

### Node Capacity (`nodes/<node-id>/capacity`)

- Updated every 15 seconds by `ploynode`.
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

### Garbage Collection (`gc/pending/<job-id>`)

- The GC controller marks jobs here before deletion to provide observability.
- After IPFS unpin succeeds and metadata is removed, the key is deleted.

## Watchers & Notifications

- Control plane services may watch specific prefixes:
  - `queue/` for new work (scheduler).
  - `nodes/<node-id>/status` for heartbeat monitoring.
  - `config/` for configuration changes.
- Ensure watchers are scoped to specific prefixes to avoid excessive load on etcd.

## Summary

- etcd keys are well-scoped by function, allowing efficient scans/watchers.
- Jobs, queue entries, and node capacity updates must use transactions to maintain consistency.
- IPFS metadata and log references stay lightweight (CIDs/digests) while payloads live in IPFS.
