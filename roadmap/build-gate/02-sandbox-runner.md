# Build Gate Sandbox Runner
- [x] Completed (2025-10-05)

## Why / What For
 Port the deterministic sandbox build flow into `internal/workflow/buildgate` so workstation runs can execute builds and capture structured results without Grid dependencies. The sandbox runner exists for RED-phase tests only; Grid remains the default execution path for real builds.

## Required Changes
- Implement `buildgate.SandboxRunner` wrapping the existing sandbox build logic with structured outputs.
- Expose CLI toggles to opt into sandbox execution while Grid wiring is still landing.
- Record build duration, cache reuse, and failure reasons for inclusion in checkpoint metadata.

## Definition of Done
- Sandbox runner executes builds locally with deterministic outputs for unit tests.
- Runner integrates sandbox results when Grid execution is unavailable.
- Unit tests cover success, timeout, and failure scenarios with coverage ≥90% across the new package.

Status: Sandbox runner landed 2025-10-05 with structured outcomes and timeout handling. Static check adapter registry (`roadmap/build-gate/03-static-check-registry.md`) and log retrieval (`roadmap/build-gate/04-log-retrieval-and-grid-integration.md`) remain active follow-ups.

## Tests
- New unit tests for `buildgate.SandboxRunner` covering cache reuse, timeouts, and error propagation.
- `go test -cover ./...` stays above repository thresholds.

## References
- Build Gate design (`docs/design/build-gate/README.md`).
- Grid Workflow RPC helper guide for workstation fallback expectations (`../grid/sdk/workflowrpc/README.md`).
