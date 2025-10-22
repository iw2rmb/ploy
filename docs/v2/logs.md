# Log Storage & Streaming

Ploy v2 keeps etcd lean by storing only log metadata in the control plane and placing the actual
log payloads in IPFS Cluster. This avoids hitting etcd’s size limits and keeps write throughput
predictable.

## Publishing Logs

- Job stdout/stderr is routed into IPFS Cluster as the job runs. Nodes stream log chunks to both
  the requester (SSE) and a rotating log file.
- When the job ends, the node finalises the log bundle, pins it in IPFS, and records the resulting
  CID in the job metadata:

```json
{
  "logs": {
    "cid": "bafy...",
    "size": 1048576,
    "content_type": "text/plain"
  }
}
```

- Only small derived fields (log digest, tail snippet, CID) live in etcd; the raw log content does
  not.

## Streaming

- `GET /v2/jobs/{id}/logs/stream` proxies live output directly from the node so operators can tail
  logs without waiting for IPFS uploads to finish.
- After completion, `GET /v2/jobs/{id}/logs` fetches from the archived bundle. The API downloads the
  content from IPFS, optionally truncating based on query parameters (e.g., `?tail=2000`).
- Node-level logs (`/v2/nodes/{node}/logs/stream`) follow the same pattern: streaming first,
  archived bundles stored via IPFS.

### CLI Streaming

- `ploy mods logs <ticket>` establishes an SSE stream against
  `/v2/mods/{ticket}/logs/stream`. The CLI defaults to a structured view (`timestamp stream line`),
  supports `--format raw` for verbatim log output, and automatically retries transient disconnects
  (`--max-retries` and `--retry-wait` tune behaviour).
- `ploy jobs follow <job-id>` tails job logs in real time using `/v2/jobs/{id}/logs/stream`, sharing
  the same formatting and retry semantics so operators can follow a single step through completion.
- When retention metadata is published on the stream (see
  [`observability-log-bundles`](../design/observability-log-bundles/README.md)), the CLI surfaces
  bundle TTLs, expiry timestamps, and archived CIDs so operators know how long the log bundle will
  remain addressable.

## Retention

- Log bundles follow the same `expires_at` lifecycle as other job artifacts (see
  [docs/v2/gc.md](gc.md)). When a job’s retention window lapses, the GC controller unpins the log
  CID and removes the reference from etcd.
- Operators can override the default retention duration per job or via `ploy gc --older-than`.

## Operational Notes

- Monitoring: track log upload latency and IPFS pin status to catch slow nodes.
- Compression: log tarballs can be gzip-compressed before upload to reduce storage costs.
- Security: ensure logs do not contain secrets; if sensitive data is present, apply redaction before
  archiving or restrict access controls on the log download endpoint.
