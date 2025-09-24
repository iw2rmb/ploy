# Mods KB Lock Operations

This runbook captures the day-to-day commands for operating the JetStream-backed Mods knowledge base locks.

## Inspect Current Locks
- List active keys: `nats kv ls mods_kb_locks --server $PLOY_JETSTREAM_URL`
- Retrieve a lock: `nats kv get mods_kb_locks writers/<kb-id>`
- Show bucket status: `nats kv info mods_kb_locks --server $PLOY_JETSTREAM_URL`

Each value stores JSON with `owner`, `lease_seconds`, `acquired_at`, and `lease_expires_at`. Missing keys indicate the lock is free.

## Manually Release a Lock
1. Grab the latest revision:
   ```bash
   REVISION=$(nats kv get mods_kb_locks writers/<kb-id> --json | jq .revision)
   ```
2. Delete using the revision guard:
   ```bash
   nats kv del mods_kb_locks writers/<kb-id> --last-revision "$REVISION"
   ```
3. Confirm release via `nats kv get` (should return `not found`).

## Subscribe to Lock Events
- Acquire events: `nats consumer add mods_kb_lock_events tail-acquired --filter "mods.kb.lock.acquired.>"`
- Release events: `nats consumer add mods_kb_lock_events tail-released --filter "mods.kb.lock.released.>"`
- Live stream: `nats consumer next mods_kb_lock_events tail-released --ack --count 10`

Event payloads include `kb_id`, `owner`, `revision`, and `lease_expires_at`. The `mods.kb.lock.expired.*` subject emits when the manager takes over a stale lock before issuing a fresh CAS.

## Troubleshooting
- **Contention**: Inspect lock acquisition failures and stream events to identify current owners.
- **Stuck Locks**: Fetch lock record and compare `lease_expires_at` with current time; use manual release if overdue.
- **Event Gaps**: Confirm `mods_kb_lock_events` stream exists and consumer cursors are current; re-create durable if necessary.
- **Fallback**: Set `PLOY_USE_JETSTREAM_KV=false` temporarily to revert to Consul, but file an incident and plan rollback ASAP.
