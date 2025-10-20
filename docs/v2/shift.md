# SHIFT Build Gate Simplification

Ploy v2 drops Grid dependencies, so the SHIFT repository must operate as a standalone build gate
module. Key changes:

## Remove Grid Integrations

- Delete Grid RPC clients, JetStream consumers, and queue listeners. SHIFT no longer receives jobs
  via Grid; Ploy v2 dispatches build-gate jobs directly.
- Remove Grid-specific environment variables (`GRID_*`, JetStream URLs) from configuration structs
  and docs.

## Expose a Clean Library API

- Keep the core build gate runner packages (sandbox, static-check adapters) but ensure they can be
  called as Go libraries with explicit inputs/outputs.
- If the SHIFT CLI remains, ensure it runs standalone (no implicit Grid bootstrap). Treat it as a
  developer tool for local testing.

## Tests

- Replace Grid-based integration tests with local ones: run the sandbox/adapter pipeline directly,
  asserting the expected outputs.
- Remove any test harnesses that spun up fake Grid APIs or JetStream fixtures.

By simplifying SHIFT this way, Ploy v2 can reuse the build gate logic via module imports without any
legacy Grid wiring.
