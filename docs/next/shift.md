# SHIFT Build Gate Simplification

Ploy Next drops legacy runtime dependencies, so the SHIFT repository must operate as a standalone build gate
module. Key changes:

## Remove Legacy Integrations

- Delete legacy RPC clients, JetStream consumers, and queue listeners. SHIFT no longer receives jobs
  via the retired stack; Ploy Next dispatches build-gate jobs directly.
- Remove legacy-specific environment variables (e.g., `GRID_*`, JetStream URLs) from configuration structs
  and docs.

## Expose a Clean Library API

- Keep the core build gate runner packages (sandbox, static-check adapters) but ensure they can be
  called as Go libraries with explicit inputs/outputs.
- If the SHIFT CLI remains, ensure it runs standalone (no implicit legacy bootstrap). Treat it as a
  developer tool for local testing.

## Tests

- Replace legacy integration tests with local ones: run the sandbox/adapter pipeline directly,
  asserting the expected outputs.
- Remove any test harnesses that spun up fake APIs or JetStream fixtures.

## Current Status (2025-10)

- Each `ployd` node runtime invokes the SHIFT sandbox after every step and surfaces the resulting
  diagnostics in job metadata.
- Static-check adapters remain disabled until artifact publishing lands in `roadmap-mod-step-runtime-03`;
  once available, the SHIFT client will attach the structured reports to the staged artifacts.

By simplifying SHIFT this way, Ploy Next can reuse the build gate logic via module imports without any
legacy wiring.
