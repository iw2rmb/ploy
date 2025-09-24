# Deploy Dependency Seams

## Purpose
Unify the `ploy` and `ployman` deployment flows behind a single dependency-injected client so the CLIs, build gates, and integration harnesses can all exercise the same HTTP and packaging logic. The seam removes tight coupling to package-level helpers (`http.DefaultClient`, `utils.GitSHA`, `utils.TarDir`) and makes it possible to provide fakes for offline tests while still defaulting to production implementations.

## Narrative Summary
- Introduce a `sharedpush` dependency bundle that centralises tar creation, SHA stamping, time handling, and HTTP transport.
- Extend `internal/cli/common` so `SharedPush` receives dependencies from callers (or uses defaults) rather than constructing them internally.
- Update the `ploy` and `ployman` handlers to populate the new configuration structure, enabling a gradual migration away from bespoke deployment code.
- Ensure tests can inject lightweight fakes to assert on outgoing requests without creating real tarballs or hitting the network.

## Key Files
- `internal/cli/common/deploy.go#L1` – Core deployment client updated to honour injected dependencies.
- `internal/cli/common/deps.go#L1` – Defines the dependency bundle (`TarBuilder`, `HTTPDoer`, `SHAResolver`, `Clock`).
- `internal/cli/deploy/handler.go#L1` / `internal/cli/platform/handler.go#L1` – CLI entry points that provide seam-aware configuration.
- `internal/cli/common/deploy_test.go#L1` – Unit coverage for dependency override behaviour and error handling.
- `roadmap/deploy/01-dependency-seams.md#L1` – Task tracker describing required changes and definition of done.

## Dependency Injection Strategy
- `DeployConfig` grows a `Deps` field (pointer) that callers may populate with the dependency bundle.
- Each dependency has an interface so tests can provide minimal fakes (`TarBuilder.Build`, `HTTPDoer.Do`).
- Default implementations live alongside the client and are used whenever a field in `Deps` is `nil`, ensuring backwards compatibility for existing callers.
- Query-string construction supports custom overrides (e.g., async, build-only) so CLI flags continue to work.

## Tests
- Unit: `go test ./internal/cli/common` (verifies seam defaults, injected HTTP client, error propagation).
- CLI handlers: `go test ./internal/cli/deploy` and `./internal/cli/platform` with injected fakes to assert lane/environment behaviour.
- Coverage: run via `mcp_golang__test_with_coverage` targeting updated packages to keep module coverage above 60% (90% on critical paths).

## Related Documentation
- `README.md#Unified Deployment Roadmap` – High-level context for the unified deployment effort.
- `docs/DOCS.md` – Documentation conventions followed when authoring this guide and roadmap entries.
- `roadmap/deploy.md` – Programme-level tracking for the deployment unification project.
