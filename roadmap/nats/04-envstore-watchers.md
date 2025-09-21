# EnvStore Watchers & Cutover

## What to Achieve
Switch env-store reads to JetStream and expose watcher-based cache invalidation that honours the NATS by Example catch-up sentinel semantics.

## Why It Matters
Real-time propagation removes polling load and ensures builds/mods see configuration changes instantly, enabling the simplifications promised in the migration plan.

## Where Changes Will Affect
- `api/consul_envstore/store.go` – read path, watcher subscription lifecycle, error handling.
- `internal/build/` and Mods wiring – adjust to subscribe to watcher channels where applicable.
- Documentation (`docs/FEATURES.md`, `api/README.md`) – describe JetStream as the authoritative env store.

## How to Implement
1. Promote JetStream as the primary source once drift checks from dual-write stage succeed; keep Consul as fallback behind a kill switch.
2. Implement watcher goroutines using `Watch()` and wait for the nil catch-up event before applying live updates.
3. Replace time-based cache invalidation with watcher-triggered updates across builders and Mods entry points.
4. Translate CAS failures into HTTP 409 responses for API clients.
5. Update READMEs/docs to reflect the new real-time propagation behaviour right after the stage.

## Expected Outcome
Env-store reads and cache busting operate solely on JetStream, providing instantaneous updates across the platform.

## Tests
- Unit: Simulate watcher events in `api/consul_envstore` tests to ensure caches refresh correctly.
- Integration: Run `go test ./api -run EnvStore` with embedded JetStream watchers enabled.
- E2E: Trigger `ploy env set` and confirm builder pods perceive the change without polling (e.g., via mods E2E scenario logs).
