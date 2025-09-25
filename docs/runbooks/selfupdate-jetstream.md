# Self-Update JetStream Runbook

Operational guide for the controller self-update work-queue (`updates.control-plane`) and status stream (`updates.control-plane.status`).

## Overview
- **Purpose**: Coordinate controller binary rollouts via JetStream work-queue semantics and publish status telemetry for CLI/UI consumers.
- **Streams**:
  - Work queue: `updates_control-plane` (subjects `updates.control-plane.tasks.*`, retention `WorkQueue`).
  - Status stream: `updates_control_plane_status` (subjects `updates.control-plane.status.*`, retention `Limits`).
- **Consumers**:
  - Executor: `updates-control-plane-<lane>` (MaxAckPending=1, DeliverPolicy=All).
  - CLI/UI: `updates-status-cli-<deployment>` (AckExplicit, DeliverAll).
- **CLI tooling**: `ploy updates tail <deployment-id>` renders live status events.

## Prerequisites
1. **Credentials**: Ensure the controller and operators have NATS creds scoped to:
   - `updates.control-plane.tasks.*` (publish/subscribe) for executor.
   - `updates.control-plane.status.*` (publish/subscribe) for status publishers and tailers.
   Mount via Nomad templates: `NATS_UPDATES_CREDS` for handlers, `PLOY_UPDATES_JETSTREAM_CREDS` for CLI or debugging shells.
2. **Environment Variables**:
   - Controller: `PLOY_UPDATES_JETSTREAM_URL`, `PLOY_UPDATES_STREAM`, `PLOY_UPDATES_SUBJECT_PREFIX`, `PLOY_UPDATES_STATUS_STREAM`, `PLOY_UPDATES_STATUS_SUBJECT_PREFIX`.
   - CLI/ops: `PLOY_UPDATES_JETSTREAM_URL` or `PLOY_JETSTREAM_URL`, optional `NATS_UPDATES_CREDS`.
3. **Nomad Deployment**: Controller job must render the JetStream creds and start with `updatesCfg.Enabled=true` (`api/server/config.go`).
4. **Binary Artifacts**: Ensure target controller versions exist under `artifacts/api-binaries/<version>/...` in object storage for executor pulls.

## Common Operations
### Inspect Queue Depth
```bash
nats stream info updates_control-plane --server "$PLOY_UPDATES_JETSTREAM_URL" --creds "$NATS_UPDATES_CREDS"
```
- `Messages` > 0 with `Consumer Count` 0 implies executor down.
- Use `nats consumer info updates_control-plane updates-control-plane-d` per lane to confirm ack lag.

### Tail Status Events
```bash
export PLOY_UPDATES_JETSTREAM_URL=nats://nats.ploy.local:4223
export NATS_UPDATES_CREDS=~/.config/ploy/updates.creds
ploy updates tail "$DEPLOYMENT_ID"
```
- `--follow` keeps streaming after terminal phases.
- Exit codes are zero on success, non-zero on connection failure or missing env vars.

### Requeue or Cancel Tasks
1. Identify stuck task:
   ```bash
   nats stream ls updates_control-plane --subjects "updates.control-plane.tasks.*"
   nats stream view updates_control-plane --id "$DEPLOYMENT_ID"
   ```
2. Force redelivery (delayed NAK) when executor recovered:
   ```bash
   nats msg nak updates_control-plane "$DEPLOYMENT_ID" --delay 30s
   ```
3. Cancel by acknowledging terminal failure manually (only if safe):
   ```bash
   nats msg ack updates_control-plane "$DEPLOYMENT_ID"
   ```

### Publish Audit Event
Use controller API (preferred) or direct JetStream publish for emergency audit trails:
```bash
nats pub updates.control-plane.audit "{\"deployment_id\":\"$DEPLOYMENT_ID\",\"action\":\"manual-override\",\"actor\":\"$USER\"}"
```

## Monitoring & Alerts
- **Metrics** (`ploy_updates_*`):
  - `ploy_updates_tasks_submitted_total` (HTTP handler success)
  - `ploy_updates_redeliveries_total` (executor NAK count)
  - `ploy_updates_status_consumer_lag_seconds` (CLI auto-export via status handler)
- **Log markers**:
  - `[selfupdate] work queue fetch error` → inspect JetStream connectivity / executor permissions.
  - `[selfupdate] nak failed task` → JetStream refused redelivery, usually due to consumer eviction.
- **Alert thresholds**:
  - Pending tasks older than 5 minutes.
  - Redelivery count > 2 for same deployment.
  - Status consumer lag > 30 seconds.

## Troubleshooting
| Symptom | Likely Cause | Resolution |
| --- | --- | --- |
| `ploy updates tail` reports "jetstream url is not configured" | Env vars missing on workstation | Export `PLOY_UPDATES_JETSTREAM_URL` or update CLI profile. |
| Work queue messages redeliver immediately | Executor crash before ack / wrong creds | Verify controller logs, rotate JetStream creds, ensure `updates-control-plane-<lane>` consumer exists. |
| Status stream empty even though deployment running | Publisher offline or wrong subject prefix | Check controller logs for `[selfupdate] publish status event failed`, confirm `PLOY_UPDATES_STATUS_SUBJECT_PREFIX`. |
| Duplicate deployment submission rejected | `ErrDuplicateTask` dedupe by `deployment_id` | Use new UUID or investigate stuck task before retrying. |

## References
- `roadmap/nats/07-selfupdate-workqueue.md` – Detailed migration plan and design context.
- `api/selfupdate/workqueue.go` – Work queue + status publisher implementation.
- `internal/cli/updates` – CLI consumer with durable pull subscribers.
