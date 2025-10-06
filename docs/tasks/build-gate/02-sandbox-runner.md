# Build Gate Sandbox Runner

- [x] Completed (2025-10-05)

## Why / What For

Port the deterministic sandbox build flow into `internal/workflow/buildgate` so
workstation runs can execute builds and capture structured results without Grid
dependencies. The sandbox runner exists for RED-phase tests only; Grid remains
the default execution path for real builds.

## Required Changes

- Implement `buildgate.SandboxRunner` wrapping the existing sandbox build logic
  with structured outputs.
- Expose CLI toggles to opt into sandbox execution while Grid wiring is still
  landing.
- Record build duration, cache reuse, and failure reasons for inclusion in
  checkpoint metadata.

## Definition of Done

- Sandbox runner executes builds locally with deterministic outputs for unit
  tests.
- Runner integrates sandbox results when Grid execution is unavailable.
- Unit tests cover success, timeout, and failure scenarios with coverage ≥90%
  across the new package.

## Current Status (2025-10-05)

- Sandbox runner ships with structured outcomes, timeout handling, and
  deterministic outputs.
- CLI toggles allow opting into sandbox execution while Grid wiring finalises.
- Follow-up slices for static check adapter registry and log retrieval continue
  separately (`docs/tasks/build-gate/03-static-check-registry.md`,
  `docs/tasks/build-gate/04-log-retrieval-and-grid-integration.md`).

## Tests

- Unit tests for `buildgate.SandboxRunner` covering cache reuse, timeouts, and
  error propagation.
- `go test -cover ./...` stays above repository thresholds.
- Practice RED → GREEN → REFACTOR: introduce failing sandbox-runner tests, add
  minimal implementation, then refactor once coverage is stable.

## References

- Build Gate design (`docs/design/build-gate/README.md`).
- Grid Workflow RPC helper guide for workstation fallback expectations
  (`../grid/sdk/workflowrpc/README.md`).
