# Cancel Endpoint Rollout Notes

This document provides rollout and operational guidance for the `POST /v1/mods/{id}/cancel` endpoint introduced in the Mods: Cancel Run feature.

## Overview

The cancel endpoint provides a control-plane API for cancelling in-flight Mods  runs. It transitions a run and any in-flight or pending stages to `canceled` status, persists an optional reason, and publishes a terminal  run event over SSE so `--follow` exits cleanly.

## Idempotency

The cancel endpoint is fully idempotent:
- If the  run is already in a terminal state (`succeeded`, `failed`, `canceled`), the endpoint returns `200 OK` without modifying state.
- If the  run is in a non-terminal state, it transitions to `canceled` and returns `202 Accepted`.
- Multiple cancel requests for the same  run are safe and will not cause errors.

This design ensures safe retry semantics and allows automation scripts to cancel  runs without checking current state first.

## Database Schema

**No database migrations required.** The cancel handler reuses existing queries:
- `UpdateRunStatus` — Transitions run to `canceled` with optional reason and timestamp.
- `ListStagesByRun` — Retrieves all stages for the run.
- `UpdateStageStatus` — Transitions each `pending` or `running` stage to `canceled`.

The implementation avoids any schema changes and works with the existing `runs` and `stages` tables.

## Authentication and Authorization

The cancel endpoint requires **mTLS with `RoleControlPlane`**. Ensure that:
- CLI admin certificates are properly issued via `/v1/pki/sign/admin` during server deployment.
- The client presents a valid certificate signed by the cluster CA.
- The certificate's Common Name matches the expected control-plane role pattern.

In production, the server enforces mTLS at the HTTP layer. Test environments may use an insecure authorizer for local development.

## Related Operations

The cancel endpoint is **symmetric** with the existing `ploy mod run` workflow:
- `ploy mod run --follow --cap <duration> --cancel-on-cap` submits a run, follows events with a timeout, and automatically cancels if the cap is exceeded.
- `ploy mod cancel --run <id> [--reason <text>]` directly cancels a run via the API.

A **resume** operation (to restart or retry a canceled  run) is tracked separately and not part of this feature.

## SSE Integration

The cancel handler publishes a terminal run event (`state=cancelled`) over SSE when the cancellation succeeds. This ensures:
- `ploy mod run --follow` receives the terminal state and exits cleanly.
- `ploy mods logs <run-id>` streams show the cancellation event.
- SSE clients observing `/v1/mods/{id}/events` receive the `cancelled` state.

Optional: The handler may also emit a `PublishStatus(done)` event for stream completion.

## Testing the Endpoint

### Smoke Test (Local or Lab)

Run a short-lived run with automatic cancellation:

```bash
ploy mod run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature \
  --follow \
  --cap 1s \
  --cancel-on-cap
```

Expected behavior:
1. Run is submitted and starts running.
2. Follow times out after 1 second.
3. CLI automatically calls `POST /v1/mods/{id}/cancel` with reason `cap exceeded`.
4. Run transitions to `canceled`.
5. SSE stream emits terminal event.
6. CLI exits cleanly.

### Manual Cancel

Submit a run without `--follow`, then cancel it manually:

```bash
# Submit a run
ploy mod run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature

# Manually cancel the run
ploy mod cancel --run <run-id> --reason "manual intervention"
```

Expected behavior:
1. Run transitions to `canceled`.
2. Optional reason is persisted in run metadata.
3. Endpoint returns `202 Accepted`.
4. Subsequent cancel requests return `200 OK`.

### Idempotency Test

Cancel the same run multiple times:

```bash
ploy mod cancel --run <run-id>
ploy mod cancel --run <run-id>
ploy mod cancel --run <run-id>
```

Expected behavior:
- First call returns `202 Accepted` (or `200 OK` if already terminal).
- Subsequent calls return `200 OK`.
- No errors or state corruption.

### Batch Run Cancellation

When canceling a batch run, all attached `run_repos` and their jobs transition to `canceled`:

```bash
# Create a batch and add repos.
ploy mod run --spec mod.yaml --name batch-to-cancel
ploy mod run repo add \
  --repo-url https://github.com/org/repo-a.git \
  --base-ref main \
  --target-ref feature \
  batch-to-cancel

ploy mod run repo add \
  --repo-url https://github.com/org/repo-b.git \
  --base-ref main \
  --target-ref feature \
  batch-to-cancel

# Cancel the entire batch (all run_repos are canceled).
ploy mod cancel --run batch-to-cancel --reason "batch aborted"
```

Expected behavior:
- The batch run and all `run_repos` transition to `canceled`.
- Jobs in `pending` or `running` state become `canceled`.
- Already-terminal jobs (succeeded/failed) remain unchanged.
- Cancellation is idempotent and can be retried safely.

See `cmd/ploy/README.md` § "Batched Mod Runs" for the full batch command reference.

### Edge Cases

Test the following edge cases:
- **Invalid run ID**: `POST /v1/mods//cancel` (empty or whitespace-only ID) → `400 Bad Request`.
- **Missing run**: `POST /v1/mods/nonexistent123/cancel` → `404 Not Found`.
- **Already succeeded**: Cancel a completed run → `200 OK`.
- **Already failed**: Cancel a failed run → `200 OK`.
- **Multiple in-flight stages**: Cancel a run with multiple `running` stages → All stages transition to `canceled`.

## Risks and Mitigations

### Race Conditions

**Risk**: A stage completes between the cancel request and the stage status update.

**Mitigation**: The handler only transitions stages in `pending` or `running` states. Terminal stages (`succeeded`, `failed`, `canceled`) are left unchanged. Database transactions ensure atomicity.

### Long-Running Transactions

**Risk**: Large  runs with hundreds of stages may cause slow cancel operations.

**Mitigation**: The current implementation updates stages individually. Future optimization: add a bulk `CancelStagesByRun` query to reduce round-trips. Monitor query performance in production.

### SSE Delivery Delays

**Risk**: SSE clients may not receive the cancellation event immediately if the connection is slow or interrupted.

**Mitigation**: Terminal events are persisted in the database before being published. Clients can always query `/v1/mods/{id}` for the latest state. The CLI's SSE client includes reconnect logic with exponential backoff.

### Unauthorized Cancellations

**Risk**: Non-control-plane clients attempt to cancel  runs.

**Mitigation**: The endpoint enforces `RoleControlPlane` authorization. Ensure mTLS is properly configured and certificates are not leaked.

## Monitoring and Observability

### Metrics

Monitor the following Prometheus metrics (if exposed by the server):
- `http_requests_total{endpoint="/v1/mods/:id/cancel", status="202"}` — Successful cancellations.
- `http_requests_total{endpoint="/v1/mods/:id/cancel", status="200"}` — Idempotent cancellations (already terminal).
- `http_requests_total{endpoint="/v1/mods/:id/cancel", status="404"}` — Missing runs.

### Logs

The server logs (via `slog`) include:
- `msg="cancel run"` with `run_id`, `requester`, and `reason` fields.
- `msg="run already terminal"` for idempotent requests.
- `msg="run not found"` for 404 responses.

Query logs with:

```bash
journalctl -u ployd.service | grep "cancel  run"
```

### Database Queries

Check canceled runs in PostgreSQL:

```sql
SELECT id, state, metadata->>'reason' AS cancel_reason, finished_at
FROM runs
WHERE state = 'canceled'
ORDER BY finished_at DESC
LIMIT 10;
```

## Rollout Checklist

Before deploying to production:

- [ ] Server binary includes the cancel handler (commit: `8df072d7` or later).
- [ ] mTLS is configured with `RoleControlPlane` for the cancel endpoint.
- [ ] Smoke test passes in lab environment with `--cap 1s --cancel-on-cap`.
- [ ] Manual cancel test verifies idempotency and terminal event delivery.
- [ ] PostgreSQL query performance is acceptable for typical  run sizes.
- [ ] Monitoring and alerting are configured for cancel endpoint metrics.
- [ ] Documentation is updated in OpenAPI spec (`docs/api/paths/mods_id_cancel.yaml`).

## References

- API Spec: `docs/api/paths/mods_id_cancel.yaml`
- Handler Implementation: `internal/server/handlers/handlers_mods_cancel.go`
- CLI Command: `internal/cli/mods/cancel.go`
- CLI Flag: `ploy mod run --help` → `--cap` and `--cancel-on-cap`
- Deployment Guide: `docs/how-to/deploy-a-cluster.md`
- Environment Variables: `docs/envs/README.md`
