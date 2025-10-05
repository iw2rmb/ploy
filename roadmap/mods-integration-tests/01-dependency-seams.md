# 01 Dependency Seams

- [x] Completed (pre-2025-09; legacy integration seam cleanup)

## Why / What For

Hard-coded helpers inside Mods tests invoke SeaweedFS, builder APIs, and Git
directly. When these endpoints are unreachable (e.g., outside the Nomad network)
the suite fails immediately. Introducing dependency seams lets us inject fakes
for hermetic runs and real clients when the harness provides them.

## Required Changes

- Define Go interfaces for artifact uploads, builder submissions, and Git pushes
  under `internal/mods` (or a dedicated dependency package).
- Implement production clients that wrap the existing HTTP/Git logic and expose
  configuration via constructors.
- Thread the interfaces through Mods runner/subsystems so code paths use
  injected dependencies instead of package-level functions.

## Definition of Done

- Mods code compiles with injected dependencies and defaults to the production
  implementations when none are provided.
- No remaining direct references to hard-coded SeaweedFS/builder/Git helpers in
  integration tests.
- Unit tests compile with fakes in place (e.g., using in-memory uploaders).

## Current Status

- Dependency seams landed prior to the SHIFT reboot; Mods tests rely on injected
  uploaders and builders with in-memory fallbacks.
- Future slices reference this baseline for workstation-only Mods testing.

## Implementation Notes

- Added `noopArtifactUploader` and memory-backed storage helpers for hermetic
  tests.
- Updated Mods runner helpers to prefer injected uploaders and adjusted tests to
  record uploads instead of stubbing HTTP globals.
- Default KB storage tests now use in-memory storage unless
  `PLOY_TEST_SEAWEEDFS` is set.

## Tests

- `go test ./internal/mods -run TestSeaweedFSKBStorage`
- `go test ./internal/mods -run TestWriteBranchChainStepMeta`
- `go test ./internal/mods -run TestSubmitPlannerJobUsesArtifactUploader`
- Maintain RED → GREEN → REFACTOR discipline even for legacy work: keep failing
  seam tests ahead of wiring, add minimal adapters, then refactor once suite
  passes.

## References

- [Design doc](../../../docs/design/mods-integration-tests/README.md)
