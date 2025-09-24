# 01 Dependency Seams

- [x] Status: Done

## Why / What For
The CLI deploy paths still construct tarballs, resolve SHAs, and perform HTTP uploads via package-level helpers. Tests must spin up full HTTP servers and write to disk, making it expensive to validate edge cases. Introducing explicit seams lets us inject fakes, share behaviour across `ploy`, `ployman`, and Mods build gates, and pave the way for eliminating duplicate deployment code.

## Required Changes
- Add a dependency bundle to `internal/cli/common` so `SharedPush` can use injected implementations for HTTP, tar creation, timestamps, and SHA resolution.
- Provide default production implementations that mirror existing behaviour when callers omit overrides.
- Update CLI handlers to populate the new configuration object and lean on the shared client instead of custom deploy logic.
- Extend unit tests to verify injected dependencies are honoured (e.g., fake HTTP client captures payloads without hitting the network).

## Definition of Done
- `SharedPush` works with both default and caller-supplied dependencies, maintaining backwards-compatible semantics.
- `ploy` and `ployman` handlers delegate to the seam and no longer rely on bespoke deployment helpers.
- Tests pass with injected fakes and no longer require writing tarballs to disk for the happy-path checks.

## Tests
- `go test ./internal/cli/common` covering seam defaults and overrides.
- `go test ./internal/cli/deploy` and `./internal/cli/platform` validating handler integration.
- `mcp_golang__test_with_coverage` to ensure coverage remains above project thresholds.

## References
- [Design doc](../../docs/design/deploy/README.md)
- [Unified deployment roadmap](../deploy.md)
