# Routing Object Store Runbook

Operational checklist for migrating routing persistence from Consul KV to the JetStream Object Store and maintaining the event-driven Traefik sync pipeline.

## Overview
- **Bucket**: `routing_maps` (JetStream object store)
- **Event Stream**: `routing_events` with subject prefix `routing.app.*`
- **Controller Metrics**: `routing_objectstore_create_total`, `ploy_api_routing_operations_total`
- **Traefik Consumer**: `routing-sync` durable pull consumer running in the Traefik sidecar job

## Prerequisites
1. **Feature Flags**
   - `PLOY_ROUTING_JETSTREAM_ENABLED=true`
2. **JetStream Credentials**
   - Controller config must surface one of: credentials file (`PLOY_ROUTING_JETSTREAM_CREDS`) or user/password pair (`PLOY_ROUTING_JETSTREAM_USER` / `_PASSWORD`).
   - Place creds alongside env-store secrets so rotation follows the same workflow.
3. **Traefik Sync Release**
   - Deploy the `cmd/traefik-sync` binary and Nomad template changes that subscribe to `routing.app.*` before enabling controller writes.
4. **Verification Harness**
   - Ensure the unit/integration tests under `internal/routing` and `api/server` are runnable from the workstation. These cover bootstrap telemetry and consumer lag handling without requiring external dashboards.

## Cutover Procedure
1. **Pre-flight Parity Check**
   ```bash
   ./cmd/ploy-migrate-routing --dry-run --manifest out/routing-manifest.json
   ```
   Validate diffs are empty. Investigate mismatches before enabling JetStream writes.
2. **Enable JetStream Persistence on the Controller**
   - Set `PLOY_ROUTING_JETSTREAM_ENABLED=true` in the controller deployment.
   - Redeploy the controller (`./bin/ployman api deploy --monitor`).
3. **Verify Bootstrap Telemetry**
   ```bash
   curl -fsS "$PLOY_CONTROLLER/metrics" | grep routing_objectstore_create_total
   ```
   Expect `status="success"` to increment once per controller instance.
4. **Monitor Event Fan-out**
   - Run targeted tests (e.g. `go test ./internal/routing -run TestStoreSaveAppRoutePersistsAndPublishes -count=1`) to confirm publish behaviour when applying changes.
   - Spot-check `/metrics` output for `routing_objectstore_create_total` and `ploy_api_routing_operations_total` counters to ensure controller instances are emitting telemetry.
5. **Confirm JetStream-Only Operation**
   - Verify `/metrics` no longer exposes any `consul_*` routing operations.
   - Remove legacy Consul keys or ACL policies if they were retained for observation.
6. **Post-Cutover Validation**
   - Trigger `ploy routing resync <app>` for a handful of apps and confirm Traefik picks up revisions immediately.
   - Run platform smoke tests that exercise custom domains to ensure routing updates converge.

## Rollback Procedure
1. **Freeze JetStream Writes**
   - Set `PLOY_ROUTING_JETSTREAM_ENABLED=false` and redeploy controller. This stops new object store writes and event publishes.
2. **Rehydrate Consul (if necessary)**
   ```bash
   ./cmd/ploy-migrate-routing --to consul --manifest out/routing-manifest.json
   ```
   Use the latest manifest to copy JetStream state back to Consul.
3. **Reset Traefik Sidecars**
   - Restart the Traefik job to clear durable consumer cursors.
   - Confirm follow-up `go test ./internal/routing -run TestRebroadcastAppPublishesEvent -count=1` passes to verify the rebroadcast path after restarts.
4. **Audit Metrics**
   - Verify `routing_objectstore_create_total{status="error"}` does not continue climbing; persistent errors require JetStream remediation before reattempting.

## Monitoring & Alerts
- **Automated Tests**: Treat the routing unit/integration suites as the primary guardrail. Incorporate them into pre-deploy checklists so regressions surface without external observability stacks.
- **Metrics Endpoint**
  ```bash
  curl -fsS "$PLOY_CONTROLLER/metrics" | grep routing_objectstore_create_total
  ```
  Verify bootstrap counters move as expected after deployments. Pair with `ploy_api_routing_operations_total` to confirm JetStream updates are flowing.
- **Logs**
  ```bash
  curl -fsS "$PLOY_CONTROLLER/platform/api/logs?lines=200" | grep "routing object store"
  ```
  Controller startup now emits success/error entries for JetStream bootstrap along with the bucket and stream identifiers.

## Troubleshooting
- **Bootstrap Errors**
  - Confirm the controller is loading the correct creds path.
  - `routing_objectstore_create_total{status="error"}` increments with accompanying log lines that include the JetStream error message.
- **Stalled Consumers**
  - Inspect `tests/e2e/deploy/fetch-logs.sh` output or the `traefik-sync` logs for ack failures.
  - Use `ploy routing resync <app>` to replay the latest object when lag persists for a single application.
- **Parity Drift**
  - Re-run the migration CLI with `--diff` to compare JetStream and Consul records.

Keep this runbook updated as routing helpers, CLI tools, or observability assets evolve.
