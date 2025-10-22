# Log Storage & Streaming

Ploy v2 keeps etcd lean by storing only log metadata in the control plane and placing the actual
log payloads in IPFS Cluster. This avoids hitting etcd’s size limits and keeps write throughput
predictable.

## Publishing Logs

- Job stdout/stderr is routed into IPFS Cluster as the job runs. Nodes stream log chunks to both
  the requester (SSE) and a rotating log file.
- When the job ends, the node finalises the log bundle, performs a deduplicated pin with retry
  backoff, and records the resulting CID, digest, and retention window in the job metadata:

```json
{
  "bundles": {
    "logs": {
      "cid": "bafy...",
      "digest": "sha256:...",
      "size": 1048576,
      "retained": true,
      "ttl": "24h",
      "expires_at": "2025-10-23T14:00:00Z"
    }
  },
  "retention": {
    "retained": true,
    "ttl": "24h",
    "expires_at": "2025-10-23T14:00:00Z",
    "bundle": "logs",
    "bundle_cid": "bafy..."
  }
}
```

The top-level `retention` block summarises the active bundle, TTL, and expiry timestamp so the CLI and
control plane APIs can surface inspection-ready jobs without re-scanning bundle metadata.

- Only small derived fields (log digest, retention TTL, CID) live in etcd; the raw log content does
  not. Duplicate payloads reuse the existing CID instead of triggering a second pin.

## Streaming

- `GET /v2/jobs/{id}/logs/stream` exposes a server-sent events (SSE) stream backed by the in-memory
  log hub. Calls may provide `Last-Event-ID` to resume from a previously observed frame.
- Node services expose the same contract at `GET /node/v2/jobs/{id}/logs/stream`, enabling CLI
  fallbacks when the control plane is unavailable.
- Streams are bounded (history of 256 frames; per-subscriber buffer of 32). Slow subscribers are
  dropped and must reconnect with `Last-Event-ID`.
- After completion, `GET /v2/jobs/{id}/logs` fetches the archived bundle from IPFS, optionally
  truncated via query parameters (e.g., `?tail=2000`).
- Node-level logs (`/v2/nodes/{node}/logs/stream`) are intended for direct operator access while
  investigating node behaviour.

### Event Frames

Streams emit structured JSON payloads per event type:

| Event       | Payload fields                                                                            |
|-------------|--------------------------------------------------------------------------------------------|
| `log`       | `timestamp`, `stream`, `line` (newline trimmed).                                           |
| `retention` | `retained`, `ttl`, `expires_at`, `bundle_cid` (omitted if retention metadata unavailable).  |
| `done`      | `status` (`completed`, `failed`, `cancelled`).                                             |

The `done` event terminates the stream and signals the CLI to stop reconnecting. Clients track the
numeric SSE id to support resumable replay.

### CLI Streaming

- `ploy mods logs <ticket>` establishes an SSE stream against
  `/v2/mods/{ticket}/logs/stream`. The CLI defaults to a structured view (`timestamp stream line`),
  supports `--format raw` for verbatim log output, and automatically retries transient disconnects
  (`--max-retries` and `--retry-wait` tune behaviour).
- `ploy jobs follow <job-id>` tails job logs in real time using `/v2/jobs/{id}/logs/stream`, sharing
  the same formatting and retry semantics so operators can follow a single step through completion.
  When a retention hint arrives, the CLI prints a `Retention: ...` summary with the bundle CID, TTL,
  and expiry window after the stream closes.
- When retention metadata is published on the stream (see
  [`observability-log-bundles`](../design/observability-log-bundles/README.md)), the CLI surfaces
  bundle TTLs, expiry timestamps, and archived CIDs so operators know how long the log bundle will
  remain addressable.

## Retention

- Log bundles follow the same `expires_at` lifecycle as other job artifacts (see
  [docs/v2/gc.md](gc.md)). When a job’s retention window lapses, the GC controller unpins the log
  CID and removes the reference from etcd. The scheduler computes `expires_at` when recording the
  bundle and publishes an aggregate `retention` summary alongside job records so downstream
  consumers do not need to re-run TTL math.
- Operators can override the default retention duration per job or via `ploy gc --older-than`.

## Operational Notes

- Monitoring: track log upload latency and IPFS pin status to catch slow nodes. Prometheus exposes
  `ploy_ipfs_bundle_pin_total{kind,result}`, `ploy_ipfs_bundle_pin_retry_total{kind}`, and
  `ploy_ipfs_bundle_pin_duration_seconds{kind}` for alerting on failures or slow pins.
- Compression: log tarballs can be gzip-compressed before upload to reduce storage costs.
- Security: ensure logs do not contain secrets; if sensitive data is present, apply redaction before
  archiving or restrict access controls on the log download endpoint.
