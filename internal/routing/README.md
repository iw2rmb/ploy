# Routing JetStream Store

Object store, stream, and consumer helpers that power Traefik’s push-based routing sync. The controller persists per-app route maps to JetStream and pushes revision events; sidecars subscribe and render dynamic config.

## Data Flow
- **Object Store**: `routing.Store` writes `apps/<app>/routes.json` into the `routing_maps` bucket (128 KiB chunking, checksum metadata).
- **Event Stream**: Every mutation publishes `routing.app.<app>` messages with revision/checksum headers on the `routing_events` stream.
- **Consumers**: Traefik sidecars run `cmd/traefik-sync` which durably pulls events, fetches the referenced object, and rewrites `/data/dynamic-config.yml` atomically.
- **Metrics**: Controller startup increments `routing_objectstore_create_total{status="success|error"}`; each save/delete increments `ploy_api_routing_operations_total{operation="jetstream_*"}`.

## Packages
- `store.go` — JetStream persistence + event emission (requires bucket/stream to exist).
- `sync/` — Sidecar helpers for rendering Traefik dynamic config from the object store (also used in tests).
- `tags.go` — Traefik tag builder used by the controller when registering services.

## Tests
Use the in-memory NATS server harness (`natstest`) for unit coverage.
```bash
go test ./internal/routing
```
Key cases:
- `store_test.go` verifies object writes and event publication (`routing.app.*`).
- `sync/event_test.go` covers durable consumer catch-up, checksum validation, and file writes.

Run alongside controller tests to exercise bootstrap telemetry:
```bash
go test ./api/server -run TraefikRouter
```

## Operational Notes
- Configure `PLOY_ROUTING_JETSTREAM_URL`, bucket (`routing_maps`), stream (`routing_events`), and subject prefix via controller env.
- Traefik sidecars need matching credentials plus `PLOY_TRAEFIK_ROUTING_DURABLE` to maintain cursor state.
- `ploy routing resync <app>` rebroadcasts the last known object via the controller using `Store.RebroadcastApp`.
- Migration tool `cmd/ploy-migrate-routing` backfills Consul data into JetStream and generates parity manifests.
