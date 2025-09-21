# Event Fabric Consolidation

## What to Achieve
Stream platform events—build lifecycle, Nomad allocation readiness, Mods telemetry—into JetStream so CLIs and services consume them via pull consumers instead of bespoke HTTP polling.

## Why It Matters
A unified event fabric simplifies observability, reduces load on the controller, and enables future automations to react to platform signals in real time.

## Where Changes Will Affect
- `internal/orchestration/` (log streamer, monitor) – publish readiness events.
- `internal/mods/events_emit.go`, controller reporters – redirect output to JetStream subjects.
- CLI/UI clients – subscribe to JetStream subjects for live updates.
- Documentation (`docs/observability.md`, CLI docs) – explain subscription patterns and troubleshooting.

## How to Implement
1. Define subject taxonomy (`alloc.ready`, `build.status`, `mods.events`) and create corresponding JetStream streams with retention policies.
2. Update emitters to publish into JetStream while preserving existing HTTP hooks during transition.
3. Refactor CLI/UI components to use pull consumers with backpressure-aware fetch loops.
4. Provide bridge tooling (e.g., HTTP SSE backed by JetStream) if backward compatibility is needed.
5. Update docs immediately after enabling each event stream to record consumption patterns and access controls.

## Expected Outcome
Platform telemetry flows through JetStream, enabling multiple subscribers to consume events reliably without controller polling.

## Tests
- Unit: Add tests for event publisher helpers ensuring correct subjects and payloads.
- Integration: Run orchestrated build/deploy tests verifying events land in JetStream streams.
- E2E: Execute Mods and build scenarios while tailing JetStream subscriptions to confirm end-to-end delivery.
