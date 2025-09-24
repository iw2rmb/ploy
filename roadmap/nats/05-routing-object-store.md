# Routing Persistence & Events

## What to Achieve
- Persist platform and custom domain routing maps in a dedicated JetStream Object Store bucket (`routing.maps`) with per-app JSON objects.
- Publish revision-aware updates on `routing.app.<app>` so Traefik sidecars and controller routines reconcile changes without polling Consul.
- Ship tooling to backfill existing Consul data, verify parity, and expose operational toggles for safe cutover and rollback.

## Why It Matters
- Object Store removes Consul's 512 KiB limit and lets us attach metadata (revision, checksum, issuer) that will support future multi-port and blue/green routing.
- Event-driven fan-out removes the Traefik polling loop; routes converge immediately after an update and reduce Consul read amplification.
- Aligns routing with the env-store JetStream work so all controller coordination (env, routing, certs) lives on a consistent fabric before retiring Consul.

## Where Changes Will Affect
- `internal/routing/kv.go`, `internal/routing/README.md` - refactor helpers into an Object Store client, add parity validation and feature flags.
- `api/routing/traefik.go`, `api/server/initializers_infra.go` - emit events when routes change, wire NATS credentials/config alongside existing Consul bootstrap.
- Traefik sidecar/bootstrap scripts (`platform/traefik/`, Helm/Nomad templates) - subscribe to `routing.app.*`, buffer updates until watcher catch-up, and swap Consul reads for streamed artifacts.
- Documentation: `docs/networking.md`, `docs/FEATURES.md`, `roadmap/README.md` - describe the new persistence model, operational toggles, and rollback.

## Prerequisites
- JetStream cluster and object-store capability deployed per `roadmap/nats/01-jetstream-cluster.md`.
- KV adapter + env-store watchers completed (stages 02-04) so shared NATS credentials + connection pooling patterns already exist.
- Feature flag scaffold that can toggle JetStream routing on/off (`PLOY_ROUTING_JETSTREAM_ENABLED`).
- Controller and Traefik sidecars trust store includes NATS TLS certs if we move to TLS-only listeners.

## Data Model & Event Contracts
- **Bucket**: `routing.maps`, history `1`, replicas `3`, max object size >= 1 MiB to cover multi-domain payloads. Objects keyed by app slug (e.g., `apps/<app>/routes.json`).
- **Payload**: JSON array of `DomainRoute` structs plus metadata block containing checksum, last writer, and `revision` (JetStream object digest). Store timestamps in RFC3339 to align with existing schema.
- **Metadata**: Use object headers for `X-Ploy-Revision`, `X-Ploy-Source` (`api` vs `migration`), and `X-Ploy-Checksum` (SHA256). These feed idempotency checks on consumers.
- **Event Subject**: `routing.app.<app>` with payload `{ "revision": <uint64>, "checksum": "...", "updated_at": "..." }`. Include `prev_revision` when present so consumers can detect stale updates.
- **Durable Consumer**: Traefik sidecars register as durable pull consumers (`routing-sync`) and ack after applying updates. Controller jobs use ephemeral consumers for manual resyncs.

## How to Implement
1. **Bootstrap Object Store**
   - Extend the controller bootstrap to create `routing.maps` if missing (`js.ObjectStore()`), aligning retention and replicas with the cluster policy.
   - Add telemetry counters (`routing_objectstore_create_total`) and error logs echoed to `platform/api` logger.
     Implemented via `metrics.RecordRoutingObjectStoreBootstrap` inside `api/server/initializers_infra.go` so bootstrap success/failure is visible in metrics and logs.
2. **Backfill & Parity Tooling**
   - Write `cmd/ploy-migrate-routing` (or extend existing migration CLI) to snapshot Consul keys (`ploy/domains/<app>`) and upload to Object Store using 128 KiB chunk writes per NATS guidance.
   - After upload, download the object, compare JSON canonicalized output, and emit diffs to stdout + metrics. Store a manifest for verification.
3. **Object Store Update Path**
   - Modify `internal/routing` helpers so `SaveAppRoute` persists exclusively to JetStream. Wrap writes in CAS using the last known digest to prevent lost updates.
   - On failures, surface structured errors and metrics counters; return errors immediately so operators can react before drift accumulates.
4. **Read & Cache Layer**
   - Introduce `GetAppRoutesJS` that streams the object in 128 KiB chunks, decodes JSON progressively (avoid loading entire payload in memory), and exposes revision/checksum to callers.
   - Remove Consul lookups from read paths; error if the object store is unavailable so issues surface immediately.
5. **Event Emission**
   - After a successful write, publish to `routing.app.<app>` with `nats.MsgHeaders` carrying revision, checksum, and `apiservice` metadata. Ensure idempotency by skipping publish if revision unchanged.
   - Integrate with existing audit/event loggers so updates show up in platform telemetry.
6. **Traefik Subscriber Rollout**
   - Ship a sidecar handler (Go helper or script) that starts a durable consumer, waits for the `nil` catch-up sentinel, fetches the referenced object, validates checksum, then updates dynamic config atomically.
   - Respect back-pressure: pause consuming when Traefik reload is in-flight; ack once reload succeeds. Persist last revision locally to bridge restarts.
7. **Cutover & Kill Switches**
   - Flip read path to JetStream once parity tests report clean results; remove Consul dual-write toggles from controller startup.
   - Document rollback: disable JetStream routing, re-run the migration tool to repopulate Consul, and redeploy the legacy controller build if required.
8. **Cleanup**
   - Remove Consul KV dependencies from routing helpers after stabilisation, delete the keys, and update ACL policies.
   - Archive migration manifests and add smoke tests to CI ensuring bucket + stream exist.

## Observability & Operations
- Keep targeted tests that publish and delete routes as the primary signal for rollout readiness (see `internal/routing/store_test.go`). Automate them in CI before toggling feature flags.
- Alerting falls back to controller logs and metrics scraping via `/metrics`; ensure on-call runbooks capture the log patterns emitted during bootstrap failures.
- Provide `ploy routing-resync <app>` CLI command invoking an ephemeral consumer to force-fetch latest revision for troubleshooting.
- Expose metrics counters (`ploy_api_routing_operations_total`, `routing_objectstore_create_total`) for JetStream operations; rely on scripted checks rather than dashboards.

## Deliverables
- Refactored routing helpers with JetStream-only persistence, feature flags, telemetry, and documentation updates in `internal/routing/README.md` & `docs/networking.md`.
- Migration CLI / script committed under `cmd/` or `scripts/` with manifest output and repeatable dry-run mode.
- Traefik sidecar update (Nomad template or container build) that consumes `routing.app.*` subjects and validates checksums before reload.
- Traefik sidecar binary (`cmd/traefik-sync`) and Nomad wiring (`platform/nomad/traefik.hcl`, `iac/common/templates/nomad-traefik-system.hcl.j2`) consuming `routing.app.*` and rewriting `/data/dynamic-config.yml`.
- Migration CLI (`cmd/ploy-migrate-routing`) to sync Consul state into JetStream with manifest output and drift detection.
- `ploy routing resync` helper wiring JetStream rebroadcasts for targeted apps.
- Operational docs describing cutover, rollback, and telemetry verification recorded in `docs/runbooks/routing-object-store.md` (new runbook outlines the migration guardrails and monitoring hooks).

## Expected Outcome
Routing metadata lives in JetStream, Traefik converges via events within seconds, and Consul KV is no longer required for routing persistence.

## Tests
- **Unit**: Extend `internal/routing` tests with in-memory JetStream (NATS server test harness) covering object uploads, CAS behaviour, and event emission.
- **Integration**: Controller integration tests that perform route updates end-to-end and assert both Object Store persistence and `routing.app.*` publish events.
- **E2E**: Deploy apps via pipeline, update custom domains, and confirm Traefik logs show receipt of matching revisions without Consul access. Include failure injection (drop consumer acks) to validate replay semantics.
