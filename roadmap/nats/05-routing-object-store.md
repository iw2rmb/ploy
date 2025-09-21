# Routing Persistence & Events

## What to Achieve
Migrate domain routing metadata from Consul to JetStream Object Store and publish `routing.app.<app>` events that Traefik sidecars consume.

## Why It Matters
Object Store enables richer metadata without size constraints, while event-driven updates eliminate the need for Traefik to poll Consul for changes.

## Where Changes Will Affect
- `internal/routing/kv.go`, `api/routing/traefik.go` – storage backend and event emission.
- Traefik sidecar configuration/scripts – subscribe to JetStream subjects for dynamic updates.
- Documentation (`docs/networking.md`, `internal/routing/README.md`) – describe the new event-driven sync path.

## How to Implement
1. Create a JetStream Object Store bucket for routing maps and migrate existing Consul data via backfill script.
2. Update save/get helpers to read/write the Object Store, streaming payloads in 128 KiB chunks per NATS example guidance.
3. Emit a message on `routing.app.<app>` whenever mappings change; include revision metadata for idempotency.
4. Modify Traefik controller/sidecars to subscribe to the subject, wait for watcher catch-up, and apply updates atomically.
5. Update related documentation after deployment describing operational procedures and rollback strategy.

## Expected Outcome
Routing metadata resides in JetStream, and Traefik updates respond immediately to published events without Consul dependencies.

## Tests
- Unit: Extend routing helper tests to cover Object Store interactions (using in-memory JetStream).
- Integration: Run controller integration tests that update routes and assert event publication.
- E2E: Deploy an app and update custom domains, verifying Traefik updates immediately via routing logs.
