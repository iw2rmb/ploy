# CLI Streaming Contract

## Overview

The CLI consumes server-sent events (SSE) emitted by both the control plane and node
services to display live job and Mods logs. Streams are exposed at:

- Control plane: `GET /v2/jobs/{id}/logs/stream`
- Nodes: `GET /node/v2/jobs/{id}/logs/stream`

Both endpoints share identical event semantics so the CLI can automatically fail
over between sources.

## Event Types

Streams emit the following event types in order:

| Event       | Description                                                                 |
|-------------|------------------------------------------------------------------------------|
| `log`       | Single log line with metadata (timestamp, stdout/stderr).                    |
| `retention` | Retention hint emitted once per stream (may be omitted for short-lived runs).|
| `done`      | Terminal status (`completed`, `failed`, `cancelled`).                        |

### `log`

```json
{
  "timestamp": "2025-10-22T13:00:00Z",
  "stream": "stdout",
  "line": "job started"
}
```

- `timestamp`: RFC3339 timestamp supplied by the producer.
- `stream`: logical channel (`stdout`, `stderr`, `system`).
- `line`: newline trimmed payload.

### `retention`

```json
{
  "retained": true,
  "ttl": "72h",
  "expires_at": "2025-10-25T13:00:00Z",
  "bundle_cid": "bafyret-logs"
}
```

- `retained`: `true` when the job is preserved for inspection.
- `ttl`: ISO-8601 duration describing retention window (`""` when unspecified).
- `expires_at`: Optional RFC3339 expiry (blank if unknown).
- `bundle_cid`: IPFS CID for archived logs (blank when not yet available).

### `done`

```json
{
  "status": "completed"
}
```

- `status`: lower case terminal state (`completed`, `failed`, `cancelled`).

The `done` event is always the final frame on the stream. When it is delivered the
connection closes.

## Last-Event-ID Behaviour

Clients may resume a stream by supplying the `Last-Event-ID` header with the
previously processed event id. The hub retains up to 256 events per stream. If
`Last-Event-ID` is older than the retention window the server replies with the
available subset and then continues streaming new frames.

The event id is a monotonically increasing integer assigned by the hub. Frames
always arrive in sequence and no gaps are introduced except when events are
pruned from history.

## Backpressure & Disconnects

- Each subscriber is assigned a bounded buffered channel (default size: 32).
- Slow consumers are dropped to prevent unbounded memory growth.
- Producers do not retry dropped deliveries; the CLI should reconnect using
  `Last-Event-ID` to fill gaps.

## CLI Expectations

The CLI parses the JSON payloads described above and prints structured output by
default (`timestamp stream line`). `--format raw` prints only the `line` value.
Retention hints are recorded during streaming and summarised when the connection
terminates.

Reconnection semantics:

1. Attempt SSE connection (max retries and backoff configurable).
2. On disconnect, reconnect with `Last-Event-ID` header containing the last id
   processed.
3. Stop retrying once the `done` event has been observed.

## Sample Stream

```
id: 1
event: log
data: {"timestamp":"2025-10-22T13:00:00Z","stream":"stdout","line":"job started"}

id: 2
event: retention
data: {"retained":true,"ttl":"72h","expires_at":"2025-10-25T13:00:00Z","bundle_cid":"bafyret-logs"}

id: 3
event: done
data: {"status":"completed"}
```

The CLI verifies the sequence (`log` → `retention` → `done`), writes log lines to
stdout, caches the retention hint, and prints a summary when the stream ends.
