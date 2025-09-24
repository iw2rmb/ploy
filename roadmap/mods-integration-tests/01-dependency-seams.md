# 01 Dependency Seams

- [ ] Status: Pending

## Why / What For
Hard-coded helpers inside Mods tests invoke SeaweedFS, builder APIs, and Git directly. When these endpoints are unreachable (e.g., outside the Nomad network) the suite fails immediately. Introducing dependency seams lets us inject fakes for hermetic runs and real clients when the harness provides them.

## Required Changes
- Define Go interfaces for artifact uploads, builder submissions, and Git pushes under `internal/mods` (or a dedicated dependency package).
- Implement production clients that wrap the existing HTTP/Git logic and expose configuration via constructors.
- Thread the interfaces through Mods runner/subsystems so code paths use injected dependencies instead of package-level functions.

## Definition of Done
- Mods code compiles with injected dependencies and defaults to the production implementations when none are provided.
- No remaining direct references to hard-coded SeaweedFS/builder/Git helpers in integration tests.
- Unit tests compile with fakes in place (e.g., using in-memory uploaders).

## Tests
- Update/extend unit tests covering the new dependency injection paths (`go test ./internal/mods/...`).
- Linting/static analysis must pass (golangci-lint, errcheck).
- Spot-check a targeted integration run using fakes to ensure behaviour parity with prior helpers.

## References
- [Design doc](../../../docs/design/mods-integration-tests/README.md)
